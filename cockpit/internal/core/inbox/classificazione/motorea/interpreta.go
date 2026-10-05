package motorea

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Interpreta e i suoi passi (5.4.6, «Interpreta, passo per passo»). Il principio che regge ogni passo
// (5.0, 04/10 sera): il motore non indovina relazioni. Due cose si collegano solo quando la provenance lo
// consente (stesso segmento, stessa entità, stessa riga di tabella, stesso nodo STEP, legame dichiarato nei
// fatti, uso esplicito) E una regola semantica lo fa nascere (una forma o una regola attiva della grammatica,
// una riga del router, una regola del piano). Senza tutte e due le cose restano separate. Ogni punto di
// collegamento qui sotto scrive nel commento quale provenance lo consente e quale regola lo fa nascere.

// Le capacità di lettura del documento (5.4.6 punto 16): sono le sole che decidono lo stato. I nomi sono quelli
// che dichiarano gli adattatori (estrazione), che motorea non importa: testo e cartiglio del PDF, struttura dello
// STEP, contenuto di un file che ha il solo nome.
var capacitaDiLettura = []string{"testo", "cartiglio", "struttura", "contenuto"}

// testoDelCorpo: il testo su cui si misura PosTabella.Esatto (la foglia lo dichiara nel commento di PosTabella).
const testoDelCorpo = "messaggio.corpo_testo"

// Le localizzazioni e i metodi delle unità che Interpreta guarda (valori chiusi della foglia, non esportati lì).
const (
	localizzazioneEsatta = "esatta"
	metodoOCR            = "ocr"
	zonaPagina           = "pagina"
)

// I motivi stabili che una lettura può portare oltre a quello del router.
const (
	motivoFonteOCR       = "fonte ocr"
	motivoFuoriZona      = "etichetta fuori dalla zona del cartiglio"
	motivoIDNomeDiscordi = "id e nome dello stesso nodo STEP danno letture diverse"
	motivoRipetizione    = "base ripetuta che non concorda"
)

// Interpreta legge un documento con l'uso dei segmenti dato (parte 1 §7.3). Non riceve i prodotti della RFQ né
// una classe desiderata: cambiare i target non cambia l'interpretazione (A-C11). L'errore è solo di contratto
// (documento o uso non validi: *evidenze.ErroreContratto con le diagnostiche documento.*), oppure di un motore
// nullo; un limite dei dati sta dentro l'Interpretazione, con lo stato parziale, mai un successo vuoto. Le
// regole che legge oltre ai piani sono quelle che CompilaVerificato ha congelato nel Motore (E4): nessuna
// lettura successiva dello snapshot del chiamante.
//
// È pura e deterministica: niente orologio, file, rete, goroutine; nessun ordine dipende da una mappa. Le unità
// si leggono in ordine di ID, e il risultato ha ogni elenco in ordine canonico.
func (m *Motore) Interpreta(doc evidenze.DocumentoEvidenze, uso evidenze.UsoSegmenti) (Interpretazione, error) {
	if m == nil {
		return Interpretazione{}, errors.New("motorea: Interpreta su un motore nullo: si costruisce solo con CompilaVerificato")
	}
	// Punto 1: la porta del contratto. Nessun ripiego: un uso sbagliato non diventa un uso sconosciuto.
	if d := append(evidenze.ValidaDocumento(doc), evidenze.ValidaUso(uso, doc)...); len(d) > 0 {
		return Interpretazione{}, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	improntaU, err := improntaUso(uso)
	if err != nil {
		d := []evidenze.Diagnostica{{
			Codice:    evidenze.CodiceDocumentoUsoNonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  "uso",
			Messaggio: "l'uso dei segmenti non ha una forma canonica: " + err.Error(),
		}}
		return Interpretazione{}, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	improntaL, err := jsoncanonico.ImprontaDi(limitiUsati{Versione: m.versioneLimiti, Riconoscimento: m.lim})
	if err != nil {
		return Interpretazione{}, fmt.Errorf("motorea: impronta dei limiti: %w", err)
	}

	it := nuovoInterprete(m, doc, uso)
	it.leggi()
	it.deduplica()
	it.alternative()
	it.idNomeDiscordi()
	it.attributi()
	it.copertura()
	it.pertinenza()

	r := Interpretazione{
		BundleID:          doc.BundleID,
		ClienteID:         m.snap.ClienteID,
		HashSnapshot:      m.snap.Hash,
		VersioneAlgoritmo: VersioneAlgoritmo,
		VersioneRouter:    VersioneRouter,
		VersioneLimiti:    m.versioneLimiti,
		ImprontaLimiti:    improntaL,
		VersioneRisultato: VersioneRisultato,
		ImprontaUso:       improntaU,
		Stato:             it.stato(),
		Letture:           it.letture,
		Attributi:         it.attr,
		Diagnostiche:      it.diag,
	}
	return chiudiInterpretazione(r)
}

// limitiUsati: i limiti che Interpreta applica, con la loro versione (R43 B). Entrano nell'identità
// dell'interpretazione con la loro impronta: la versione dice quale taratura, i valori che cosa si è applicato.
// Versione e valori sono quelli dei limiti che CompilaVerificato ha ricevuto e portato nel motore (E4): la
// versione è sempre quella dei valori.
type limitiUsati struct {
	Versione       string                          `json:"versione_limiti"`
	Riconoscimento grammatica.LimitiRiconoscimento `json:"riconoscimento"`
}

// identita: gli ingressi dell'interpretazione, la cui impronta è l'ID (par.3.4.2). Non c'è lo snapshot intero:
// c'è il suo hash; e c'è la versione del router, che nello snapshot non sta (R41 c).
type identita struct {
	BundleID          string `json:"bundle_id"`
	ClienteID         string `json:"cliente_id"`
	HashSnapshot      string `json:"hash_snapshot"`
	VersioneAlgoritmo string `json:"versione_algoritmo"`
	VersioneRouter    string `json:"versione_router"`
	VersioneRisultato int    `json:"versione_risultato"`
	VersioneLimiti    string `json:"versione_limiti"`
	ImprontaLimiti    string `json:"impronta_limiti"`
	ImprontaUso       string `json:"impronta_uso"`
}

// chiudiInterpretazione mette ogni elenco in ordine canonico e calcola ID e Impronta (punto 17).
func chiudiInterpretazione(r Interpretazione) (Interpretazione, error) {
	sort.SliceStable(r.Letture, func(i, j int) bool { return r.Letture[i].ID < r.Letture[j].ID })
	sort.SliceStable(r.Attributi, func(i, j int) bool { return r.Attributi[i].ID < r.Attributi[j].ID })
	sort.SliceStable(r.Diagnostiche, func(i, j int) bool {
		return chiaveDiagnostica(r.Diagnostiche[i]) < chiaveDiagnostica(r.Diagnostiche[j])
	})
	if len(r.Letture) == 0 {
		r.Letture = nil
	}
	if len(r.Attributi) == 0 {
		r.Attributi = nil
	}
	if len(r.Diagnostiche) == 0 {
		r.Diagnostiche = nil
	}
	id, err := jsoncanonico.ImprontaDi(identita{
		BundleID:          r.BundleID,
		ClienteID:         r.ClienteID.String(),
		HashSnapshot:      r.HashSnapshot,
		VersioneAlgoritmo: r.VersioneAlgoritmo,
		VersioneRouter:    r.VersioneRouter,
		VersioneRisultato: r.VersioneRisultato,
		VersioneLimiti:    r.VersioneLimiti,
		ImprontaLimiti:    r.ImprontaLimiti,
		ImprontaUso:       r.ImprontaUso,
	})
	if err != nil {
		return Interpretazione{}, fmt.Errorf("motorea: identità dell'interpretazione: %w", err)
	}
	r.ID = id
	r.Impronta = ""
	imp, err := jsoncanonico.ImprontaDi(r)
	if err != nil {
		// Il documento è già passato dalla porta della foglia (UTF-8, niente decimali): qui non dovrebbe succedere.
		return Interpretazione{}, fmt.Errorf("motorea: impronta dell'interpretazione: %w", err)
	}
	r.Impronta = imp
	return r, nil
}

// chiaveDiagnostica: l'ordine canonico delle diagnostiche, (codice, percorso, riferimenti) e poi il messaggio. I
// campi si separano con il byte 0 e i riferimenti con il byte 1: gli ID locali non hanno caratteri di controllo.
func chiaveDiagnostica(d evidenze.Diagnostica) string {
	return d.Codice + "\x00" + d.Percorso + "\x00" + strings.Join(d.Rif, "\x01") + "\x00" + d.Messaggio
}

// ImprontaUso: l'impronta canonica di un UsoSegmenti (parte 1 §9.3). Anche «sconosciuto» ha la sua. Le
// selezioni sono un insieme per segmento: il loro ordine non conta. Un uso con un testo che non è UTF-8 non ha
// una forma canonica: l'impronta è vuota, e Interpreta lo rifiuta come errore di contratto.
func ImprontaUso(u evidenze.UsoSegmenti) string {
	s, err := improntaUso(u)
	if err != nil {
		return ""
	}
	return s
}

func improntaUso(u evidenze.UsoSegmenti) (string, error) {
	c := u
	c.Selezioni = append([]evidenze.SelezioneSegmento(nil), u.Selezioni...)
	sort.SliceStable(c.Selezioni, func(i, j int) bool { return c.Selezioni[i].SegmentoID < c.Selezioni[j].SegmentoID })
	if len(c.Selezioni) == 0 {
		c.Selezioni = nil
	}
	return jsoncanonico.ImprontaDi(c)
}

// ---- lo stato di un'interpretazione in corso ----

// interprete: lo stato di Interpreta mentre legge un documento. Le mappe servono solo a cercare per ID, mai a
// scorrere: ogni elenco segue l'ordine delle unità per ID.
type interprete struct {
	m         *Motore
	doc       evidenze.DocumentoEvidenze
	regole    regoleDiInterpretazione
	testi     map[string]string
	entita    map[string]evidenze.EntitaLocale
	unitaPer  map[string]evidenze.UnitaEvidenza
	selezioni map[string]evidenze.SelezioneSegmento
	valutato  bool

	unita   []evidenze.UnitaEvidenza // le unità lette, in ordine di ID, entro max_unita_documento
	letture []LetturaCodice
	attr    []AttributoLetto
	diag    []evidenze.Diagnostica
	limite  bool // un limite superato: il risultato è parziale

	// lette: le unità lette per intero (punto 2), senza un limite che le abbia tagliate. Gli attributi che
	// dipendono dalle letture (revisione in campo separato, quantità) guardano solo entità e tabelle lette per
	// intero: su letture tagliate la regola si applicherebbe a famiglie e righe con codice che non sono tutte.
	lette map[string]bool

	// pertinenzaIgnota: i segmenti (o le unità senza segmento) da cui vengono richieste con l'uso non noto o non
	// confermato (daConfermare).
	pertinenzaIgnota []string
}

func nuovoInterprete(m *Motore, doc evidenze.DocumentoEvidenze, uso evidenze.UsoSegmenti) *interprete {
	it := &interprete{
		m:         m,
		doc:       doc,
		regole:    m.regole,
		testi:     map[string]string{},
		entita:    map[string]evidenze.EntitaLocale{},
		unitaPer:  map[string]evidenze.UnitaEvidenza{},
		selezioni: map[string]evidenze.SelezioneSegmento{},
		valutato:  uso.Stato == "valutato",
		lette:     map[string]bool{},
	}
	for _, t := range doc.Testi {
		it.testi[t.ID] = t.Testo
	}
	for _, e := range doc.Entita {
		it.entita[e.ID] = e
	}
	for _, s := range uso.Selezioni {
		it.selezioni[s.SegmentoID] = s
	}
	unita := append([]evidenze.UnitaEvidenza(nil), doc.Unita...)
	sort.SliceStable(unita, func(i, j int) bool { return unita[i].ID < unita[j].ID })
	for _, u := range unita {
		it.unitaPer[u.ID] = u
	}
	// Punto 2: le unità per documento. Oltre il limite si leggono le prime, in ordine di ID.
	if massimo := m.lim.MaxUnitaDocumento; massimo > 0 && len(unita) > massimo {
		it.superato("limiti.riconoscimento.max_unita_documento",
			fmt.Sprintf("documento di %d unità, max_unita_documento dell'indice è %d: lette le prime %d in ordine di ID, risultato parziale", len(unita), massimo, massimo))
		unita = unita[:massimo]
	}
	it.unita = unita
	return it
}

// superato: un limite dell'indice superato (R43 B). La costante è di grammatica, l'avviso di natura limite.
func (it *interprete) superato(percorso, messaggio string) {
	it.limite = true
	it.diag = append(it.diag, evidenze.Diagnostica{
		Codice:    grammatica.CodiceLimiteSuperato,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaLimite,
		Percorso:  percorso,
		Messaggio: messaggio,
	})
}

// regoleDiInterpretazione: ciò che Interpreta legge della grammatica oltre ai piani: i ruoli delle famiglie,
// per l'intersezione; le regole di revisione in campo separato riservate, che non hanno piani ma danno lo stato
// «non_interpretabile» (Q1); le regole quantita_tabellare attive, per selettore.
//
// Le calcola CompilaVerificato una volta sola, dalla stessa grammatica normalizzata da cui compila i piani, e
// stanno nel Motore (E4): piani e regole d'interpretazione dello stesso snapshot nascono e si congelano insieme.
// Sono copie: nessuna slice del chiamante, quindi una grammatica cambiata dopo la compilazione non cambia
// l'interpretazione, e nessuna guardia a ogni chiamata. Le regole restano per selettore: lo stesso cliente può
// avere forme diverse nel corpo, nel nome del file e nel cartiglio, e ognuna ha le sue regole, mai una regola
// sola per tutti i selettori né una fusione per somiglianza.
type regoleDiInterpretazione struct {
	ruoli     map[string][]grammatica.Ruolo
	riservate map[evidenze.Selettore][]revisioneRiservata
	quantita  map[evidenze.Selettore][]quantitaAttiva
}

// revisioneRiservata: una regola di revisione in campo separato con stato «riservata», con la sua famiglia.
type revisioneRiservata struct{ famiglia, regola string }

// quantitaAttiva: una regola quantita_tabellare attiva, con i suoi letterali.
type quantitaAttiva struct {
	id           string
	intestazioni []string
}

// regoleDa: le regole d'interpretazione di una grammatica già normalizzata (CompilaVerificato), in ordine di ID
// qualunque sia l'ordine del file. Ogni slice è copiata.
func regoleDa(g grammatica.Grammatica) regoleDiInterpretazione {
	r := regoleDiInterpretazione{
		ruoli:     map[string][]grammatica.Ruolo{},
		riservate: map[evidenze.Selettore][]revisioneRiservata{},
		quantita:  map[evidenze.Selettore][]quantitaAttiva{},
	}
	for _, f := range g.Famiglie {
		r.ruoli[f.ID] = append([]grammatica.Ruolo(nil), f.Ruoli...)
		for _, rv := range f.Revisioni {
			if rv.Stato == grammatica.StatoAttiva || rv.Sorgente != grammatica.SorgenteCampoSeparato {
				continue
			}
			for _, s := range rv.Selettori {
				if sel, err := evidenze.LeggiSelettore(s); err == nil {
					r.riservate[sel] = append(r.riservate[sel], revisioneRiservata{famiglia: f.ID, regola: rv.ID})
				}
			}
		}
	}
	for _, q := range g.Quantita {
		if q.Stato != grammatica.StatoAttiva || q.Regola != grammatica.RegolaQuantitaPrimaRigaSopra {
			continue
		}
		for _, s := range q.Selettori {
			if sel, err := evidenze.LeggiSelettore(s); err == nil {
				r.quantita[sel] = append(r.quantita[sel], quantitaAttiva{id: q.ID, intestazioni: append([]string(nil), q.Intestazioni...)})
			}
		}
	}
	return r
}

// ---- punti 3-8: riconoscimento, posizioni, funzione, categorie, trasformazioni, qualità ----

// leggi riconosce ogni unità, in ordine di ID, e costruisce le letture con funzione, ruoli, trasformazioni e
// qualità. Oltre max_letture_documento si fermano le letture: quelle già fatte restano (punto 2).
func (it *interprete) leggi() {
	massimo := it.m.lim.MaxLettureDocumento
	for _, u := range it.unita {
		forme, d := it.m.Riconosci(u.Selettore, u.Testo)
		tagliata := false
		for _, x := range d {
			// Le diagnostiche del riconoscimento passano avanti con l'unità fra i riferimenti.
			x.Rif = append(append([]string(nil), x.Rif...), u.ID)
			if x.Codice == grammatica.CodiceLimiteSuperato {
				it.limite = true
				tagliata = true
			}
			it.diag = append(it.diag, x)
		}
		if massimo > 0 && len(it.letture)+len(forme) > massimo {
			resta := massimo - len(it.letture)
			it.superato("limiti.riconoscimento.max_letture_documento",
				fmt.Sprintf("più di %d letture nel documento (max_letture_documento dell'indice): restano le prime %d, in ordine di unità e di posizione; le unità dopo %q non si leggono, risultato parziale", massimo, massimo, u.ID))
			for _, f := range forme[:resta] {
				it.letture = append(it.letture, it.lettura(u, f))
			}
			return
		}
		for _, f := range forme {
			it.letture = append(it.letture, it.lettura(u, f))
		}
		if !tagliata {
			it.lette[u.ID] = true
		}
	}
}

// lettura costruisce la lettura del codice di una forma riconosciuta su un'unità.
func (it *interprete) lettura(u evidenze.UnitaEvidenza, f LetturaForma) LetturaCodice {
	l := LetturaCodice{
		ID:         fmt.Sprintf("l:%s:%s/%s:%d-%d", u.ID, f.Famiglia, f.Forma, f.Intervallo.Inizio, f.Intervallo.Fine),
		UnitaID:    u.ID,
		Occorrenza: f.Intervallo,
		Assoluto:   assoluto(u, f.Intervallo),
		Forma:      f,
		Categorie:  copiaCategorie(f.Categorie), // punto 6: annotazione dalla famiglia, mai da un riconoscitore sul nome (R7)
	}

	// Punto 5: la funzione. Provenance: il segmento dell'unità (UnitaEvidenza.SegmentoID) o, per una cella,
	// quello della sua entità «riga» (EntitaLocale.SegmentoID, legami-1), con l'uso esplicito di quel segmento
	// (UsoSegmenti). Regola: la riga del router che Instrada sceglie; un uso pertinente di origine
	// «riconoscimento» è un candidato, e vale come da valutare (E2).
	seg, usoSeg, origine := it.usoDi(u)
	funz, motivo := Instrada(Instradamento{Selettore: u.Selettore, Uso: usoSeg, Origine: origine})
	l.Funzione, l.Uso, l.OrigineUso = funz, usoSeg, origine
	l.RuoliCandidati, l.MotivoRuoli = intersecaRuoli(it.regole.ruoli[f.Famiglia], funz)
	l.Motivi = []string{motivo}
	if funz == FunzRichiesta && daConfermare(usoSeg, origine) {
		dove := seg
		if dove == "" {
			dove = u.ID
		}
		if !contiene(it.pertinenzaIgnota, dove) {
			it.pertinenzaIgnota = append(it.pertinenzaIgnota, dove)
		}
	}

	l.Trasformazioni = trasformazioni(f)

	// Punto 8: la qualità, dallo stato della lettura di forma. Il router non guarda zona né fonte OCR: la lettura
	// ne porta il motivo, e per l'OCR la qualità da verificare (5.4.6, nota al router; 5.10 n.6).
	switch f.Stato {
	case StatoCompleta:
		l.Qualita = QualitaCompleta
	case StatoParziale:
		l.Qualita = QualitaParziale
	default: // discordante, da_verificare
		l.Qualita = QualitaDaVerificare
	}
	if f.Stato == StatoDiscordante {
		l.Motivi = append(l.Motivi, motivoRipetizione)
		it.diag = append(it.diag, evidenze.Diagnostica{
			Codice:    CodiceMotoreRipetizioniDiscordanti,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  "letture[" + l.ID + "]",
			Messaggio: "la base ripetuta non concorda con la base: la lettura resta, da verificare, senza scegliere la prima (D2)",
			Rif:       []string{l.ID, u.ID},
		})
	}
	if u.Qualita.Metodo == metodoOCR {
		l.Qualita = QualitaDaVerificare
		l.Motivi = append(l.Motivi, motivoFonteOCR)
	}
	if u.Selettore.Contesto == evidenze.ContestoCartiglio && u.Posizione.PDF != nil && u.Posizione.PDF.Zona == zonaPagina {
		l.Motivi = append(l.Motivi, motivoFuoriZona)
	}
	return l
}

// usoDi: il segmento di un'unità di oggetto, corpo o storia, con l'uso e l'origine della sua selezione. Per una
// cella il segmento è quello della sua riga (legami-1: la cella appartiene alla riga, la riga al segmento). Un
// segmento senza selezione, o un uso sconosciuto, è «sconosciuto»; le unità fuori da un messaggio non hanno uso.
func (it *interprete) usoDi(u evidenze.UnitaEvidenza) (segmento, uso, origine string) {
	switch u.Selettore.Contesto {
	case evidenze.ContestoOggetto, evidenze.ContestoCorpo, evidenze.ContestoStoria:
	default:
		return "", UsoNonApplicabile, ""
	}
	segmento = it.segmentoDi(u)
	if segmento == "" || !it.valutato {
		return segmento, UsoSconosciuto, ""
	}
	s, ok := it.selezioni[segmento]
	if !ok {
		return segmento, UsoSconosciuto, ""
	}
	return segmento, s.Uso, s.Origine
}

// segmentoDi: il segmento dell'unità, o quello della sua entità (una cella di una riga di tabella).
func (it *interprete) segmentoDi(u evidenze.UnitaEvidenza) string {
	if u.SegmentoID != "" {
		return u.SegmentoID
	}
	if u.EntitaID != "" {
		return it.entita[u.EntitaID].SegmentoID
	}
	return ""
}

// assoluto: l'occorrenza sul testo originale (punto 4), solo se l'unità è localizzata in modo esatto su un testo
// del documento: un intervallo di testo, il nome del file, una cella con l'aggancio esatto. Per STEP e PDF il
// testo dell'unità è la stringa dei fatti, e l'intervallo relativo basta.
func assoluto(u evidenze.UnitaEvidenza, iv evidenze.Intervallo) *evidenze.PosTesto {
	if u.Qualita.Localizzazione != localizzazioneEsatta {
		return nil
	}
	var id string
	var inizio int
	p := u.Posizione
	switch {
	case p.Tipo == "testo" && p.Testo != nil:
		id, inizio = p.Testo.TestoID, p.Testo.Intervallo.Inizio
	case p.Tipo == "nome_file" && p.NomeFile != nil:
		id, inizio = p.NomeFile.Campo, p.NomeFile.Intervallo.Inizio
	case p.Tipo == "tabella" && p.Tabella != nil && p.Tabella.Esatto != nil:
		id, inizio = testoDelCorpo, p.Tabella.Esatto.Inizio
	default:
		return nil
	}
	return &evidenze.PosTesto{TestoID: id, Intervallo: evidenze.Intervallo{Inizio: inizio + iv.Inizio, Fine: inizio + iv.Fine}}
}

// trasformazioni: una per ogni parte che resta fuori dalla base, più la normalizzazione delle maiuscole della
// base, con gli intervalli originali sul testo dell'unità (punto 7). L'originale non si perde: il testo
// nell'intervallo è sempre Prima.
func trasformazioni(f LetturaForma) []Trasformazione {
	var out []Trasformazione
	parte := func(op, regola string, p *ParteLetta, motivo string) {
		if p != nil {
			out = append(out, Trasformazione{Regola: regola, Operazione: op, Prima: p.Originale, Motivo: motivo,
				Intervalli: []evidenze.Intervallo{p.Intervallo}})
		}
	}
	parte(OpSeparaEtichetta, "", f.Etichetta, "etichetta fuori dal codice (A-C08)")
	for _, a := range f.Affissi {
		out = append(out, Trasformazione{Regola: a.Regola, Operazione: OpSeparaAffisso, Prima: a.Originale,
			Motivo: "affisso " + a.Posizione + ", fuori dalla base (parte 1 §5.3)", Intervalli: []evidenze.Intervallo{a.Intervallo}})
	}
	parte(OpSeparaMarcatore, "", f.Marcatore, "marcatore dopo la base, conservato (D1)")
	parte(OpSeparaToken, "", f.Token, "token conservato, mai attribuito (R16)")
	for _, d := range f.Decorazioni {
		out = append(out, Trasformazione{Regola: d.Regola, Operazione: OpSeparaDecorazione, Prima: d.Originale,
			Motivo: "decorazione «" + d.Tipo + "», fuori dall'identità", Intervalli: []evidenze.Intervallo{d.Intervallo}})
	}
	if r := f.Revisione; r != nil {
		out = append(out, Trasformazione{Regola: r.Regola, Operazione: OpSeparaRevisione, Prima: r.Originale,
			Dopo: r.Normalizzata, Motivo: "revisione in linea, separata dalla base (stato «" + r.Stato + "»)",
			Intervalli: []evidenze.Intervallo{r.Intervallo}})
	}
	for _, rp := range f.Ripetizioni {
		if iv, ok := intervalloBase(rp.Base); ok {
			out = append(out, Trasformazione{Operazione: OpSeparaRipetizione, Prima: rp.Base.Originale,
				Motivo: "base ripetuta, confrontata con la base (D2)", Intervalli: []evidenze.Intervallo{iv}})
		}
	}
	if f.Base.Normalizzata != f.Base.Originale {
		if iv, ok := intervalloBase(f.Base); ok {
			out = append(out, Trasformazione{Operazione: OpNormalizzaMaiuscolo, Prima: f.Base.Originale,
				Dopo: f.Base.Normalizzata, Motivo: "normalizzazione dichiarata della base: maiuscole ASCII",
				Intervalli: []evidenze.Intervallo{iv}})
		}
	}
	return out
}

// intervalloBase: l'intervallo della base sul testo dell'unità. La base finisce con il suo ultimo segmento (i
// separatori della base stanno prima dei segmenti), quindi comincia len(Originale) byte prima.
func intervalloBase(b BaseLetta) (evidenze.Intervallo, bool) {
	if len(b.Segmenti) == 0 {
		return evidenze.Intervallo{}, false
	}
	fine := b.Segmenti[len(b.Segmenti)-1].Intervallo.Fine
	inizio := fine - len(b.Originale)
	if inizio < 0 {
		return evidenze.Intervallo{}, false
	}
	return evidenze.Intervallo{Inizio: inizio, Fine: fine}, true
}

// ---- punto 9: la deduplica della stessa occorrenza ----

// deduplica unisce le letture della STESSA occorrenza (A-C10; P1 §7.3). Mai per sola stringa: lo stesso codice
// in due unità diverse (id e nome di un nodo, due righe di tabella, il nome e il cartiglio) resta due letture.
// Due letture sono la stessa occorrenza solo se hanno lo stesso contenuto (famiglia, forma, base normalizzata,
// marcatore, revisione, parti) e:
//
//	(a) lo stesso testo originale e lo stesso intervallo assoluto: una cella agganciata in modo esatto e il
//	    segmento che la contiene. Provenance: il testo originale del documento e l'intervallo esatto che
//	    l'adattatore ha dichiarato (PosTabella.Esatto, PosTesto). Regola: punto 9, primo caso;
//	(b) una cella agganciata solo per righe e una lettura del segmento della sua riga, con lo stesso testo, che
//	    cade dentro le righe della cella. Provenance: lo stesso segmento (EntitaLocale.SegmentoID della riga e
//	    UnitaEvidenza.SegmentoID) e le righe del testo che l'adattatore ha dichiarato (PosTabella.RigheTesto).
//	    Quelle righe sono righe del corpo della mail, cioè del testo dell'unità del segmento che ha il selettore
//	    della cella (corpo o storia): valgono solo lì. L'oggetto sta nello stesso segmento s:corrente, ma su un
//	    altro testo, e le righe della cella non dicono niente di lui. Regola: punto 9, secondo caso. Si uniscono
//	    solo le coppie in cui l'una è la sola candidata dell'altra: con due candidate la scelta sarebbe
//	    indovinata, e le letture restano separate.
//
// Resta una lettura sola: la principale è quella dell'unità con l'entità (la cella), le altre unità vanno in
// AltreUnita.
func (it *interprete) deduplica() {
	tolte := map[int]bool{}

	// (a) Stesso testo originale, stesso intervallo assoluto, stesso contenuto.
	gruppi := map[string][]int{}
	var chiavi []string
	for i, l := range it.letture {
		if l.Assoluto == nil {
			continue
		}
		k := fmt.Sprintf("%s\x00%d\x00%d\x00%s\x00%s", l.Assoluto.TestoID, l.Assoluto.Intervallo.Inizio, l.Assoluto.Intervallo.Fine,
			l.Forma.Originale, chiaveContenuto(l.Forma))
		if _, ok := gruppi[k]; !ok {
			chiavi = append(chiavi, k)
		}
		gruppi[k] = append(gruppi[k], i)
	}
	for _, k := range chiavi {
		if g := gruppi[k]; len(g) > 1 {
			it.unisci(g, tolte)
		}
	}

	// (b) Una cella agganciata per righe e una lettura del segmento della sua riga.
	type coppia struct{ cella, seg int }
	var candidate []coppia
	for i, c := range it.letture {
		if tolte[i] || c.Assoluto != nil {
			continue
		}
		uc := it.unitaPer[c.UnitaID]
		pt := uc.Posizione.Tabella
		if pt == nil || pt.RigheTesto == nil || uc.EntitaID == "" {
			continue
		}
		segCella := it.entita[uc.EntitaID].SegmentoID
		if segCella == "" {
			continue
		}
		for j, s := range it.letture {
			if j == i || tolte[j] {
				continue
			}
			us := it.unitaPer[s.UnitaID]
			// Lo stesso selettore della cella: le righe della cella sono righe del testo del corpo (o della
			// storia) di quel segmento, mai dell'oggetto, che pure sta in s:corrente.
			if us.SegmentoID != segCella || us.Selettore != uc.Selettore || us.Posizione.Tabella != nil ||
				s.Forma.Originale != c.Forma.Originale ||
				chiaveContenuto(s.Forma) != chiaveContenuto(c.Forma) {
				continue
			}
			if it.dentroLeRighe(us, s.Occorrenza, *pt.RigheTesto) {
				candidate = append(candidate, coppia{cella: i, seg: j})
			}
		}
	}
	perCella, perSeg := map[int]int{}, map[int]int{}
	for _, c := range candidate {
		perCella[c.cella]++
		perSeg[c.seg]++
	}
	for _, c := range candidate {
		if perCella[c.cella] == 1 && perSeg[c.seg] == 1 && !tolte[c.cella] && !tolte[c.seg] {
			it.unisci([]int{c.cella, c.seg}, tolte)
		}
	}

	if len(tolte) == 0 {
		return
	}
	resto := it.letture[:0:0]
	for i, l := range it.letture {
		if !tolte[i] {
			resto = append(resto, l)
		}
	}
	it.letture = resto
}

// unisci tiene una lettura del gruppo, quella dell'unità con l'entità (a parità, l'unità con l'ID minore), e
// porta le unità delle altre in AltreUnita. Le altre si segnano come tolte.
func (it *interprete) unisci(gruppo []int, tolte map[int]bool) {
	principale := gruppo[0]
	for _, i := range gruppo[1:] {
		a, b := it.letture[i], it.letture[principale]
		conEntitaA, conEntitaB := it.unitaPer[a.UnitaID].EntitaID != "", it.unitaPer[b.UnitaID].EntitaID != ""
		if (conEntitaA && !conEntitaB) || (conEntitaA == conEntitaB && a.UnitaID < b.UnitaID) {
			principale = i
		}
	}
	p := &it.letture[principale]
	for _, i := range gruppo {
		if i == principale {
			continue
		}
		o := it.letture[i]
		p.AltreUnita = append(p.AltreUnita, o.UnitaID)
		p.AltreUnita = append(p.AltreUnita, o.AltreUnita...)
		tolte[i] = true
	}
	sort.Strings(p.AltreUnita)
}

// chiaveContenuto: ciò che due letture della stessa occorrenza devono avere uguale (punto 9): famiglia, forma,
// base normalizzata, marcatore, revisione e parti (affissi, decorazioni, token, etichetta), con i testi letti.
func chiaveContenuto(f LetturaForma) string {
	var b strings.Builder
	scrivi := func(s string) { b.WriteString(s); b.WriteByte(0) }
	scrivi(f.Famiglia)
	scrivi(f.Forma)
	scrivi(f.Base.Normalizzata)
	for _, p := range []*ParteLetta{f.Marcatore, f.Token, f.Etichetta} {
		if p != nil {
			scrivi(p.Valore)
		} else {
			scrivi("\x01")
		}
	}
	if r := f.Revisione; r != nil {
		scrivi(r.Regola + "\x01" + r.Originale + "\x01" + r.Normalizzata + "\x01" + r.Stato)
	} else {
		scrivi("\x01")
	}
	for _, a := range f.Affissi {
		scrivi(a.Regola + "\x01" + a.Originale)
	}
	scrivi("\x02")
	for _, d := range f.Decorazioni {
		scrivi(d.Regola + "\x01" + d.Originale)
	}
	return b.String()
}

// dentroLeRighe: l'occorrenza di una lettura del segmento cade dentro le righe [da, a) del testo su cui il
// segmento è localizzato. Le righe hanno la semantica del taglio della mail (5.4.3 punto 1: CRLF un solo
// terminatore, CR o LF da soli un terminatore), ripetuta qui perché motorea non importa classificazione. La
// posizione si legge dal PosTesto del segmento anche quando la localizzazione è parziale (testo ricavato
// dall'HTML), ma solo se il testo dell'unità è davvero quello dell'intervallo: altrimenti niente.
func (it *interprete) dentroLeRighe(us evidenze.UnitaEvidenza, occ evidenze.Intervallo, righe [2]int) bool {
	pt := us.Posizione.Testo
	if us.Posizione.Tipo != "testo" || pt == nil {
		return false
	}
	testo, ok := it.testi[pt.TestoID]
	iv := pt.Intervallo
	if !ok || iv.Inizio < 0 || iv.Fine > len(testo) || iv.Inizio > iv.Fine || testo[iv.Inizio:iv.Fine] != us.Testo {
		return false
	}
	pos := righeDelTesto(testo)
	if righe[0] < 0 || righe[1] > len(pos) || righe[0] >= righe[1] {
		return false
	}
	da, a := pos[righe[0]][0], pos[righe[1]-1][1]
	inizio, fine := iv.Inizio+occ.Inizio, iv.Inizio+occ.Fine
	return inizio >= da && fine <= a
}

// righeDelTesto: [inizio, fine) di ogni riga, senza il terminatore, come strings.Split del testo normalizzato
// (5.4.3 punto 1).
func righeDelTesto(s string) [][2]int {
	var out [][2]int
	inizio := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			fine := i
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			out = append(out, [2]int{inizio, fine})
			inizio = i + 1
		case '\n':
			out = append(out, [2]int{inizio, i})
			inizio = i + 1
		}
	}
	return append(out, [2]int{inizio, len(s)})
}

// ---- punti 10 e 11: alternative, annidate, conflitti fra id e nome ----

// alternative: le letture che si sovrappongono sulla stessa unità restano tutte (punto 10). Provenance: la
// stessa unità e intervalli che si toccano; nessun legame nasce, la diagnostica dice solo che ci sono due
// letture dello stesso tratto.
//   - Famiglie diverse, o la stessa famiglia con letture diverse dello stesso tratto: motore.letture_alternative.
//   - Una lettura contenuta in un'altra della stessa famiglia: motore.letture_annidate (R25 f).
func (it *interprete) alternative() {
	idx := make([]int, len(it.letture))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := it.letture[idx[a]], it.letture[idx[b]]
		if x.UnitaID != y.UnitaID {
			return x.UnitaID < y.UnitaID
		}
		return x.ID < y.ID
	})
	for a := 0; a < len(idx); a++ {
		for b := a + 1; b < len(idx); b++ {
			x, y := it.letture[idx[a]], it.letture[idx[b]]
			if x.UnitaID != y.UnitaID {
				break
			}
			ox, oy := x.Occorrenza, y.Occorrenza
			if !(ox.Inizio < oy.Fine && oy.Inizio < ox.Fine) {
				continue
			}
			// Ogni Diagnostica nasce con la costante del suo codice scritta nel letterale (A1a-CAT).
			d := evidenze.Diagnostica{
				Codice:    CodiceMotoreLettureAlternative,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  "unita[" + x.UnitaID + "]",
				Messaggio: "due letture sovrapposte sulla stessa unità: restano tutte e due, nessuna si sceglie",
				Rif:       []string{x.ID, y.ID},
			}
			contenuta := (ox.Inizio <= oy.Inizio && oy.Fine <= ox.Fine) || (oy.Inizio <= ox.Inizio && ox.Fine <= oy.Fine)
			if x.Forma.Famiglia == y.Forma.Famiglia && contenuta && ox != oy {
				d = evidenze.Diagnostica{
					Codice:    CodiceMotoreLettureAnnidate,
					Gravita:   evidenze.GravitaAvviso,
					Natura:    evidenze.NaturaDati,
					Percorso:  "unita[" + x.UnitaID + "]",
					Messaggio: "una lettura contenuta in un'altra della stessa famiglia: restano tutte e due (R25 f)",
					Rif:       []string{x.ID, y.ID},
				}
			}
			it.diag = append(it.diag, d)
		}
	}
}

// idNomeDiscordi: se l'id e il nome dello stesso nodo STEP danno letture diverse, restano tutte e due, con la
// qualità da verificare (punto 11; P1 §10.5). Provenance: lo stesso nodo (EntitaID delle due unità). Regola:
// punto 11. Un nome senza letture non è un conflitto: è una descrizione.
func (it *interprete) idNomeDiscordi() {
	type perNodo struct{ id, nome []int }
	nodi := map[string]*perNodo{}
	var ordine []string
	for i, l := range it.letture {
		u := it.unitaPer[l.UnitaID]
		if u.EntitaID == "" || u.Selettore.Campo.Variante != evidenze.VarianteStep {
			continue
		}
		n, ok := nodi[u.EntitaID]
		if !ok {
			n = &perNodo{}
			nodi[u.EntitaID] = n
			ordine = append(ordine, u.EntitaID)
		}
		switch u.Selettore.Campo.Valore {
		case "id":
			n.id = append(n.id, i)
		case "nome":
			n.nome = append(n.nome, i)
		}
	}
	sort.Strings(ordine)
	for _, e := range ordine {
		n := nodi[e]
		if len(n.id) == 0 || len(n.nome) == 0 || stesseLetture(it.letture, n.id, n.nome) {
			continue
		}
		var rif []string
		for _, i := range append(append([]int(nil), n.id...), n.nome...) {
			l := &it.letture[i]
			l.Qualita = QualitaDaVerificare
			l.Motivi = append(l.Motivi, motivoIDNomeDiscordi)
			rif = append(rif, l.ID)
		}
		sort.Strings(rif)
		it.diag = append(it.diag, evidenze.Diagnostica{
			Codice:    CodiceMotoreIDNomeDiscordi,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  "entita[" + e + "]",
			Messaggio: "l'id e il nome dello stesso nodo STEP danno letture diverse: restano tutte, da verificare",
			Rif:       append([]string{e}, rif...),
		})
	}
}

// stesseLetture: le due unità danno le stesse letture, come insiemi di (famiglia, base normalizzata, marcatore,
// revisione).
func stesseLetture(l []LetturaCodice, a, b []int) bool {
	chiavi := func(idx []int) []string {
		var out []string
		for _, i := range idx {
			f := l[i].Forma
			k := f.Famiglia + "\x00" + f.Base.Normalizzata + "\x00"
			if f.Marcatore != nil {
				k += f.Marcatore.Valore
			}
			k += "\x00"
			if f.Revisione != nil {
				k += f.Revisione.Stato + "\x01" + f.Revisione.Normalizzata
			}
			if !contiene(out, k) {
				out = append(out, k)
			}
		}
		sort.Strings(out)
		return out
	}
	x, y := chiavi(a), chiavi(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// ---- punti 13, 15, 16 ----

// Punto 13, le relazioni: in A1 nessun codice (E3 = A; par.12). Nascono solo da una lettura con funzione
// relazione (riga 12 del router), cioè da una forma attiva su cartiglio.particolare_simile, che in A1 è
// riservato in ogni grammatica: Interpretazione.Relazioni resta vuoto. Il runtime nascerà quando una forma
// attiva potrà davvero produrle.

// copertura: per un selettore presente nel documento senza nessuna forma attiva e senza revisioni in campo
// separato, una sola nota per selettore, con le unità in Rif: «campo ricevuto, non letto» (punto 15; v3 §2).
// Non è un silenzio, e non vuol dire «nessun codice». La gravità è «nota», come la chiama il 5.4.6, non l'avviso
// con cui grammatica la emette per un elemento riservato del file regole (lo dice codici_diagnostica.go).
func (it *interprete) copertura() {
	perSelettore := map[evidenze.Selettore][]string{}
	var ordine []evidenze.Selettore
	for _, u := range it.unita {
		if len(it.m.piani[u.Selettore]) > 0 || len(it.m.revisioniCampo[u.Selettore]) > 0 {
			continue
		}
		if _, ok := perSelettore[u.Selettore]; !ok {
			ordine = append(ordine, u.Selettore)
		}
		perSelettore[u.Selettore] = append(perSelettore[u.Selettore], u.ID)
	}
	sort.SliceStable(ordine, func(i, j int) bool { return ordine[i].String() < ordine[j].String() })
	for _, s := range ordine {
		it.diag = append(it.diag, evidenze.Diagnostica{
			Codice:    grammatica.CodiceCapacitaNonSupportata,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaCapacita,
			Percorso:  "selettore[" + s.String() + "]",
			Messaggio: fmt.Sprintf("campo ricevuto, non letto: nessuna forma attiva e nessuna revisione in campo separato sul selettore %q", s.String()),
			Rif:       perSelettore[s],
		})
	}
}

// pertinenza: una nota per segmento da cui vengono richieste con la pertinenza non nota o non confermata (riga 2
// del router; daConfermare): la richiesta è da confermare, e il servizio di proposta non la tratta come
// confermata (P1 §4.3).
func (it *interprete) pertinenza() {
	dove := append([]string(nil), it.pertinenzaIgnota...)
	sort.Strings(dove)
	for _, s := range dove {
		it.diag = append(it.diag, evidenze.Diagnostica{
			Codice:    CodiceMotorePertinenzaIgnota,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  "segmento[" + s + "]",
			Messaggio: "richieste da un segmento con la pertinenza non nota o non confermata (uso sconosciuto, da valutare, o pertinente per riconoscimento automatico): da confermare",
			Rif:       []string{s},
		})
	}
}

// stato: completa, parziale o non disponibile (punto 16).
//   - non_disponibile: nessuna unità con un testo da leggere;
//   - parziale: una capacità di lettura parziale o assente, un limite superato, un'unità che il worker ha
//     troncato;
//   - completa: altrimenti. Le capacità che dicono che cosa il documento non sa distinguere (firma, sezione
//     tecnica, storia annidata, segmentazione, tabelle, elenco PDF, grafo completo) non contano: restano nella
//     qualità della fonte (R32 b).
//
// Un PDF senza testo ha il nome leggibile: è parziale, con la capacità «testo» assente, diverso da «completa con
// zero letture» (A-C09).
func (it *interprete) stato() string {
	leggibili := false
	troncate := false
	for _, u := range it.doc.Unita {
		if u.Testo != "" {
			leggibili = true
		}
		if u.Qualita.Troncata {
			troncate = true
		}
	}
	if !leggibili {
		return StatoInterpretazioneNonDisponibile
	}
	if it.limite || troncate {
		return StatoInterpretazioneParziale
	}
	for _, c := range it.doc.Qualita.Capacita {
		if contiene(capacitaDiLettura, c.Nome) && c.Stato != "disponibile" {
			return StatoInterpretazioneParziale
		}
	}
	return StatoInterpretazioneCompleta
}
