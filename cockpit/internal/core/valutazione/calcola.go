package valutazione

import (
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// calcola.go: il punto d'ingresso del percorso puro (piano A, 6.4.6, la sequenza di Calcola, passi 1–11; par.3.3.8;
// contratto §2.3, §4 riga 6.4.6; fase 0 di B6, F.2, F.3, IM.1; T-B6-09, F0-03, F0-04, F0-05). Banco e anteprima lo
// chiamano con la stessa fotografia, le stesse regole e lo stesso file dei casi, e hanno lo stesso esito (R42 B).

// Calcola è il percorso puro comune: dalla fotografia già chiusa, con le regole dei clienti e gli ingressi dei casi,
// l'esito di ogni thread e dei messaggi fuori RFQ dei casi di censimento. Non riceve mai l'atteso (R2), e un problema di
// un thread diventa una diagnostica o un thread non valutato, senza fermare gli altri.
//  1. ValidaFotografia: con una diagnostica di gravità errore, l'errore di contratto, con un esito vuoto; le altre
//     diagnostiche vanno in Esito.Diagnostiche. Un thread o un messaggio fuori RFQ ripetuto nella fotografia è anche lui
//     un errore di contratto (contratto.id_ripetuto: R-66 della revisione di V1), perché l'esito dipenderebbe
//     dall'ordine. Poi ImprontaFotografia: un payload senza forma canonica è un errore di contratto
//     (contratto.json_non_valido).
//  2. Per ogni thread, in ordine di ID (esitoDelThread): il motore della grammatica del cliente (MotoreDi, con la
//     ragione sociale della fotografia), il caso del thread nel file dei casi, ValutaProdotti, poi il vecchio e i record
//     piatti di ogni allegato. Senza motore il thread non è valutato, con il motivo; un errore della valutazione lo rende
//     non valutato con il motivo errore_valutazione (T-B6-09, F0-05).
//  3. I messaggi fuori RFQ dei casi di censimento (R34), in Esito.FuoriRFQ.
//  4. Esito.Impronta (IM.1), per ultima.
//
// r nil vuol dire nessun insieme di regole: ogni thread ha Valutato falso, con il motivo senza_grammatica_a e la nota
// regole.assenti, e l'impronta dell'indice e la versione dei limiti restano vuote. L'errore è solo di contratto
// (*evidenze.ErroreContratto).
//
// È pura e deterministica: niente DB, file, orologio, rete, goroutine; l'ordine degli elenchi della fotografia e dei
// casi non conta, e gli ingressi di chi chiama non cambiano.
func Calcola(f fotorfq.Fotografia, r *motorea.InsiemeRegole, in Ingressi) (Esito, error) {
	var errori, avvisi []evidenze.Diagnostica
	for _, d := range fotorfq.ValidaFotografia(f) {
		if d.Gravita == evidenze.GravitaErrore {
			errori = append(errori, d)
		} else {
			avvisi = append(avvisi, d)
		}
	}
	errori = append(errori, ripetuti(f)...)
	if len(errori) > 0 {
		return Esito{}, &evidenze.ErroreContratto{Diagnostiche: errori}
	}
	impronta, err := fotorfq.ImprontaFotografia(f)
	if err != nil {
		return Esito{}, &evidenze.ErroreContratto{Diagnostiche: []evidenze.Diagnostica{{
			Codice:    grammatica.CodiceJSONNonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  "fotografia",
			Messaggio: "l'impronta della fotografia non si calcola: " + err.Error(),
		}}}
	}
	e := Esito{VersioneValutazione: VersioneValutazione, VersioneImprontaProdotto: VersioneImprontaProdotto,
		VersioneFormati2D: VersioneFormati2D, VersioneComposizione: motorea.VersioneComposizione, ImprontaFotografia: impronta}
	if r != nil {
		e.ImprontaIndice, e.VersioneLimiti = r.ImprontaIndice, r.Indice.Limiti.Versione
	}

	thread := append([]fotorfq.Thread(nil), f.Thread...)
	sort.SliceStable(thread, func(i, j int) bool { return thread[i].ID.String() < thread[j].ID.String() })
	for _, t := range thread {
		e.Thread = append(e.Thread, esitoDelThread(f, t, r, in))
	}
	fuori, d := fuoriRFQ(f, r, in)
	e.FuoriRFQ = fuori
	avvisi = append(avvisi, d...)
	ordinaDiagnostiche(avvisi)
	e.Diagnostiche = avvisi

	h, err := improntaEsito(e)
	if err != nil {
		// Non succede con i tipi dell'esito (nessun decimale, nessuna mappa): se succede, l'esito non ha un'impronta e non
		// si restituisce, perché un'impronta non si inventa.
		return Esito{}, &evidenze.ErroreContratto{Diagnostiche: []evidenze.Diagnostica{{
			Codice:    CodiceErroreValutazione,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  "impronta",
			Messaggio: "l'impronta dell'esito non si calcola: " + err.Error(),
		}}}
	}
	e.Impronta = h
	return e, nil
}

// esitoDelThread: un thread, con le regole del suo cliente e il suo caso (6.4.6 passi 2–10; T-B6-09).
//   - Senza motore: Valutato falso con il motivo della grammatica; ValutaProdotti si chiama lo stesso, e i prodotti sono
//     informazione (lo stato non_pronto).
//   - Con un errore della valutazione: nessun nuovo (né richiesta né prodotti né ancoraggi, né associazioni né file da
//     smistare: dubbio T-B6-62; il fascicolo non calcolato, con thread_non_valutato); il motivo è errore_valutazione
//     se il motore c'è, altrimenti quello della grammatica; l'errore va fra le diagnostiche (diagnosticheDellErrore). I
//     record dei file e il vecchio ci sono lo stesso: i file si leggono uno per uno (nuovoCollegamento), e un errore su un
//     file lascia fuori solo quel file.
func esitoDelThread(f fotorfq.Fotografia, t fotorfq.Thread, r *motorea.InsiemeRegole, in Ingressi) EsitoThread {
	et := EsitoThread{ThreadID: t.ID, ClienteID: t.ClienteID}
	m, motivo, diag := motoreDel(f, r, t.ClienteID)
	if m != nil {
		et.HashSnapshot = m.Snapshot().Hash
	}
	v, col, err := valutaThread(f, t, m, in.CasoDelThread(t.ID))
	if err != nil {
		diag = append(diag, diagnosticheDellErrore(err, "thread["+t.ID.String()+"]", t.ID.String())...)
		if motivo == "" {
			motivo = MotivoThreadErroreValutazione
		}
		v = ValutazioneProdotti{ThreadID: t.ID}
		col = nuovoCollegamento(t, m)
	} else {
		et.Richiesta, et.Prodotti, et.Ancoraggi = v.Richiesta, v.Prodotti, v.Ancoraggi
		et.ProdottiValutati = v.ProdottiValutati
		et.Associazioni, et.DaSmistare = v.Associazioni, v.DaSmistare
		et.Conflitti = componiConflitti(v.Conflitti, v.ConflittiSmistamento)
		diag = append(diag, v.Diagnostiche...)
	}
	et.Valutato = m != nil && err == nil
	et.Motivo = motivo
	et.File = fileInterpretati(t.Allegati, col)
	diag = append(diag, diagnosticheDeiFile(col)...)
	et.Evidenze = evidenzeDelThread(col, v.Ancoraggi)
	p := nuoviPiatti(t, m, et.Valutato, col, v)
	conf, d := p.confrontabili(t)
	et.Confrontabili = conf
	diag = append(diag, d...)
	et.VecchiProdotti = vecchiProdotti(t)
	et.ProdottiConfrontabili = prodottiConfrontabili(v.Prodotti)
	// B6, V3: il fascicolo (Fascicolo, senza il gesto del modello nuovo: LD-23), con gli orfani di V2 come avviso (R93,
	// T-E1-16, F0-18). Un thread non valutato, anche per un errore, non si congela (thread_non_valutato: T-B6-09); con un
	// errore i target non si conoscono, e il fascicolo non è calcolato (dubbio T-B6-86), come con T-12. Senza grammatica
	// non è calcolato con lo stesso criterio della pertinenza (pertinenzaCalcolata: gli orfani non si calcolano), e il
	// motivo resta thread_non_valutato (R-86 della controprova di V3, che chiude T-B6-152).
	et.Fascicolo = Fascicolo(IngressoFascicolo{Valutato: et.Valutato, Calcolato: err == nil && m != nil && !fascicoloNonDeterminabile(f.Sezioni),
		Prodotti: et.ProdottiValutati, Legacy: congelamentoLegacy(t), FaseThread: t.Stato, DaSmistare: et.DaSmistare}, nil)
	ordinaDiagnostiche(diag)
	et.Diagnostiche = diag
	return et
}

// motoreDel: il motore della grammatica di un cliente, con il motivo per cui manca e le diagnostiche di MotoreDi (6.4.6
// passo 2; T-B6-09). Il motivo segue MotoreDi: un cliente con il motore ma la ragione sociale discorde; un cliente
// scartato; un cliente senza voce nell'indice. Senza insieme di regole, la nota regole.assenti come per un cliente senza
// voce.
func motoreDel(f fotorfq.Fotografia, r *motorea.InsiemeRegole, cliente uuid.UUID) (*motorea.Motore, MotivoThread, []evidenze.Diagnostica) {
	if r == nil {
		return nil, MotivoThreadSenzaGrammatica, []evidenze.Diagnostica{{
			Codice:    motorea.CodiceRegoleAssenti,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Messaggio: "nessun insieme di regole: non si valuta, nessuna regola A",
			Rif:       []string{cliente.String()},
		}}
	}
	m, d := r.MotoreDi(cliente, ragioneSociale(f, cliente))
	switch {
	case m != nil:
		return m, "", d
	case r.Motori[cliente] != nil:
		return nil, MotivoThreadRagioneSocialeDiscorde, d
	}
	if _, scartato := r.Scartati[cliente]; scartato {
		return nil, MotivoThreadGrammaticaScartata, d
	}
	return nil, MotivoThreadSenzaGrammatica, d
}

// ragioneSociale: la ragione sociale del cliente nella fotografia; "" se il cliente non c'è (allora MotoreDi la dice
// discorde: non si valuta con una grammatica che non si può controllare).
func ragioneSociale(f fotorfq.Fotografia, cliente uuid.UUID) string {
	for _, c := range f.Clienti {
		if c.ID == cliente {
			return c.RagioneSociale
		}
	}
	return ""
}

// ripetuti: un thread o un messaggio fuori RFQ che compare più volte nella fotografia (R-66 della revisione di V1): un
// errore di contratto, con il codice della grammatica per un ID ripetuto fra gli elementi dello stesso tipo
// (contratto.id_ripetuto). ValidaFotografia non lo controlla, e fotorfq è chiuso; il caricatore toglie i doppioni, un
// export potrebbe non farlo. In ordine canonico.
func ripetuti(f fotorfq.Fotografia) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	thread := map[uuid.UUID]int{}
	for _, t := range f.Thread {
		thread[t.ID]++
		if thread[t.ID] == 2 {
			out = append(out, evidenze.Diagnostica{Codice: grammatica.CodiceIDRipetuto, Gravita: evidenze.GravitaErrore, Natura: evidenze.NaturaContratto,
				Percorso: "thread[" + t.ID.String() + "]", Messaggio: "il thread compare più volte nella fotografia: l'esito dipenderebbe dall'ordine",
				Rif: []string{t.ID.String()}})
		}
	}
	messaggi := map[uuid.UUID]int{}
	for _, mf := range f.FuoriRFQ {
		id := mf.Messaggio.ID
		messaggi[id]++
		if messaggi[id] == 2 {
			out = append(out, evidenze.Diagnostica{Codice: grammatica.CodiceIDRipetuto, Gravita: evidenze.GravitaErrore, Natura: evidenze.NaturaContratto,
				Percorso: "fuori_rfq[" + id.String() + "]", Messaggio: "il messaggio fuori RFQ compare più volte nella fotografia: l'esito dipenderebbe dall'ordine",
				Rif: []string{id.String()}})
		}
	}
	ordinaDiagnostiche(out)
	return out
}

// diagnosticheDeiFile: l'errore dell'adattatore (estrazione.DaAllegato) o dell'interpretazione su un file che resta fuori
// (R-61 della revisione di V1; F0-05; 6.4.6, l'intestazione: «un problema diventa una diagnostica», e il passo 3), in
// ordine di allegato: se l'errore porta un errore di contratto, le sue diagnostiche; altrimenti
// valutazione.errore_valutazione. La gravità è avviso: l'errore tocca un file solo, il thread prosegue, e il file resta
// documento_non_leggibile (MotivoFile*). La natura resta quella della diagnostica d'origine (dati per
// errore_valutazione). Il percorso comincia con «file[<id>]», e i riferimenti hanno l'allegato.
func diagnosticheDeiFile(col *collegamento) []evidenze.Diagnostica {
	if col == nil {
		return nil
	}
	var ids []uuid.UUID
	for id := range col.errori {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	var out []evidenze.Diagnostica
	for _, id := range ids {
		err, percorso := col.errori[id], "file["+id.String()+"]"
		var ec *evidenze.ErroreContratto
		if !errors.As(err, &ec) || len(ec.Diagnostiche) == 0 {
			out = append(out, evidenze.Diagnostica{Codice: CodiceErroreValutazione, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
				Percorso:  percorso,
				Messaggio: "il file non si legge o non si interpreta, quindi resta fuori (documento_non_leggibile): " + err.Error(),
				Rif:       []string{id.String()}})
			continue
		}
		for _, d := range ec.Diagnostiche {
			d.Gravita = evidenze.GravitaAvviso
			if d.Percorso == "" {
				d.Percorso = percorso
			} else {
				d.Percorso = percorso + "." + d.Percorso
			}
			rif := append([]string(nil), d.Rif...)
			if !contiene(rif, id.String()) {
				rif = append(rif, id.String())
			}
			d.Rif = rif
			out = append(out, d)
		}
	}
	return out
}

// diagnosticheDellErrore: l'errore della valutazione di un thread o di un messaggio come diagnostiche (F0-05): se porta
// un errore di contratto, le sue diagnostiche; altrimenti valutazione.errore_valutazione con il testo dell'errore (gli
// errori della valutazione dicono gli ID, non i testi del cliente).
func diagnosticheDellErrore(err error, percorso string, rif ...string) []evidenze.Diagnostica {
	var ec *evidenze.ErroreContratto
	if errors.As(err, &ec) && len(ec.Diagnostiche) > 0 {
		return append([]evidenze.Diagnostica(nil), ec.Diagnostiche...)
	}
	return []evidenze.Diagnostica{{
		Codice:    CodiceErroreValutazione,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaDati,
		Percorso:  percorso,
		Messaggio: "la valutazione non è riuscita, quindi non è valutato: " + err.Error(),
		Rif:       rif,
	}}
}

// fileInterpretati: un record per allegato, in ordine di AllegatoID (F0-03): il file letto e interpretato dal
// collegamento, oppure, per un allegato che l'adattatore lascia fuori, il record con il motivo (contenitore, natura che non
// è un file, documento che non si legge).
func fileInterpretati(allegati []fotorfq.Allegato, col *collegamento) []FileInterpretato {
	ordinati := append([]fotorfq.Allegato(nil), allegati...)
	sort.SliceStable(ordinati, func(i, j int) bool { return ordinati[i].ID.String() < ordinati[j].ID.String() })
	letti := map[uuid.UUID]ancoraggio.FileInterpretato{}
	for _, f := range col.file {
		letti[f.AllegatoID] = f
	}
	contenitori := contenitoriDi(allegati)
	var out []FileInterpretato
	for _, a := range ordinati {
		if f, ok := letti[a.ID]; ok {
			out = append(out, FileInterpretato{AllegatoID: f.AllegatoID, Documento: f.Documento, Interpretazione: f.Interpretazione,
				Disponibilita: f.Disponibilita, Disegno: f.Disegno})
			continue
		}
		motivo := MotivoFileDocumentoNonLeggibile
		switch {
		case contenitori[a.ID]:
			motivo = MotivoFileContenitore
		case a.Natura != "" && a.Natura != naturaFile:
			motivo = MotivoFileNaturaNonFile
		}
		out = append(out, FileInterpretato{AllegatoID: a.ID, Motivo: motivo})
	}
	return out
}

// evidenzeDelThread: le evidenze del nuovo (EvidenzaFile), per ogni file ancorato in ordine di AllegatoID e, dentro, per
// ogni lettura d'identità del file nel suo ordine: l'unità, il suo testo, l'intervallo della lettura sul testo
// dell'unità, il selettore e il localizzatore.
func evidenzeDelThread(col *collegamento, a ancoraggio.EsitoAncoraggi) []EvidenzaFile {
	letti := map[uuid.UUID]ancoraggio.FileInterpretato{}
	for _, f := range col.file {
		letti[f.AllegatoID] = f
	}
	ancorati := append([]ancoraggio.AncoraggioFile(nil), a.File...)
	sort.SliceStable(ancorati, func(i, j int) bool { return ancorati[i].AllegatoID.String() < ancorati[j].AllegatoID.String() })
	var out []EvidenzaFile
	for _, af := range ancorati {
		f, ok := letti[af.AllegatoID]
		if !ok {
			continue
		}
		unita := make(map[string]evidenze.UnitaEvidenza, len(f.Documento.Unita))
		for _, u := range f.Documento.Unita {
			unita[u.ID] = u
		}
		for _, l := range lettureDiIdentita(f, af) {
			u := unita[l.UnitaID]
			out = append(out, EvidenzaFile{AllegatoID: af.AllegatoID, UnitaID: l.UnitaID, LetturaID: l.ID, Testo: u.Testo,
				Intervallo: l.Occorrenza, Selettore: u.Selettore, Posizione: u.Posizione})
		}
	}
	return out
}

// fuoriRFQ: i messaggi fuori RFQ dei casi di censimento (R34; 6.4.6, «FuoriRFQ»): per ogni caso senza thread, ogni suo
// messaggio, cercato fra i messaggi fuori RFQ della fotografia, valutato con la grammatica del cliente del caso, in ordine
// di (Caso, MessaggioID). Un messaggio del caso che la fotografia non ha è un record non valutato con
// valutazione.errore_valutazione; un messaggio fuori RFQ della fotografia che nessun caso senza thread elenca non si
// valuta, e lo dice un avviso fra le diagnostiche dell'esito, con il suo codice (valutazione.messaggio_fuori_rfq_senza_caso:
// R-63 della revisione di V1; mai silenzio).
func fuoriRFQ(f fotorfq.Fotografia, r *motorea.InsiemeRegole, in Ingressi) ([]EsitoFuoriRFQ, []evidenze.Diagnostica) {
	messaggi := map[uuid.UUID]fotorfq.MessaggioFuoriRFQ{}
	for _, mf := range f.FuoriRFQ {
		if _, ok := messaggi[mf.Messaggio.ID]; !ok {
			messaggi[mf.Messaggio.ID] = mf
		}
	}
	var out []EsitoFuoriRFQ
	elencati := map[uuid.UUID]bool{}
	for i := range in.Casi {
		caso := in.Casi[i]
		if caso.ThreadID != nil {
			continue
		}
		visti := map[uuid.UUID]bool{}
		for _, id := range caso.Messaggi {
			if visti[id] {
				continue
			}
			visti[id] = true
			elencati[id] = true
			mf, ok := messaggi[id]
			if !ok {
				out = append(out, EsitoFuoriRFQ{Caso: caso.ID, MessaggioID: id, ClienteID: caso.ClienteID, Motivo: MotivoThreadErroreValutazione,
					Diagnostiche: diagnosticheDellErrore(fmt.Errorf("valutazione: il messaggio %s del caso %q non è fra i messaggi fuori RFQ della fotografia", id, caso.ID),
						"fuori_rfq["+id.String()+"]", id.String())})
				continue
			}
			out = append(out, valutaFuoriRFQ(f, r, &caso, mf))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Caso != out[j].Caso {
			return out[i].Caso < out[j].Caso
		}
		return out[i].MessaggioID.String() < out[j].MessaggioID.String()
	})
	var diag []evidenze.Diagnostica
	for _, mf := range f.FuoriRFQ {
		if id := mf.Messaggio.ID; !elencati[id] {
			elencati[id] = true
			diag = append(diag, evidenze.Diagnostica{
				Codice:   CodiceMessaggioFuoriRFQSenzaCaso,
				Gravita:  evidenze.GravitaAvviso,
				Natura:   evidenze.NaturaDati,
				Percorso: "fuori_rfq[" + id.String() + "]",
				Messaggio: "un messaggio fuori RFQ della fotografia che nessun caso senza thread (di censimento) del file dei casi elenca: " +
					"non si valuta; fra i fuori RFQ vanno solo i messaggi dei casi senza thread",
				Rif: []string{id.String()},
			})
		}
	}
	return out, diag
}

// valutaFuoriRFQ: un messaggio fuori RFQ di un caso di censimento, valutato come un thread di un messaggio solo, con il
// cliente del caso: il motore e il motivo come per un thread; il documento del messaggio, la richiesta del messaggio con
// il gesto 1 e i segmenti del caso (RichiestaDelThread), l'interpretazione con il suo uso, i prodotti che la mail chiede
// (ProponiProdotti); i file letti e interpretati come quelli di un thread (nuovoCollegamento), con l'uso sconosciuto.
// Nessun target e nessun ancoraggio: il messaggio non ha una RFQ.
func valutaFuoriRFQ(f fotorfq.Fotografia, r *motorea.InsiemeRegole, caso *IngressoCaso, mf fotorfq.MessaggioFuoriRFQ) EsitoFuoriRFQ {
	ef := EsitoFuoriRFQ{Caso: caso.ID, MessaggioID: mf.Messaggio.ID, ClienteID: caso.ClienteID}
	m, motivo, diag := motoreDel(f, r, caso.ClienteID)
	if m != nil {
		ef.HashSnapshot = m.Snapshot().Hash
	}
	solo := fotorfq.Thread{ClienteID: caso.ClienteID, Messaggi: []fotorfq.Messaggio{mf.Messaggio},
		Agganci: []fotorfq.AggancioMessaggio{mf.Aggancio}, Allegati: mf.Allegati, Fatti: mf.Fatti}
	col := nuovoCollegamento(solo, m)
	ef.File = fileInterpretati(mf.Allegati, col)
	diag = append(diag, diagnosticheDeiFile(col)...)
	msg, prodotti, err := valutaMessaggio(solo, m, caso, mf.Messaggio)
	if err != nil {
		diag = append(diag, diagnosticheDellErrore(err, "fuori_rfq["+mf.Messaggio.ID.String()+"]", mf.Messaggio.ID.String())...)
		if motivo == "" {
			motivo = MotivoThreadErroreValutazione
		}
	} else {
		ef.Messaggio, ef.Prodotti = msg, prodotti
	}
	ef.Valutato = m != nil && err == nil
	ef.Motivo = motivo
	ordinaDiagnostiche(diag)
	ef.Diagnostiche = diag
	return ef
}

// valutaMessaggio: il documento del messaggio, la sua interpretazione con l'uso dei segmenti del caso e i prodotti
// candidati della mail. Senza motore l'interpretazione è vuota (solo il bundle) e i prodotti non hanno candidati.
func valutaMessaggio(solo fotorfq.Thread, m *motorea.Motore, caso *IngressoCaso, msg fotorfq.Messaggio) (MessaggioInterpretato, ancoraggio.EsitoProdotti, error) {
	d, err := estrazione.DaMessaggio(msg)
	if err != nil {
		return MessaggioInterpretato{}, ancoraggio.EsitoProdotti{}, fmt.Errorf("valutazione: messaggio fuori RFQ %s: %w", msg.ID, err)
	}
	richiesta, usi := RichiestaDelThread(solo, caso, map[uuid.UUID]evidenze.DocumentoEvidenze{msg.ID: d})
	mi := MessaggioInterpretato{MessaggioID: msg.ID, Documento: d, Interpretazione: motorea.Interpretazione{BundleID: d.BundleID}}
	var interpretati []ancoraggio.MessaggioInterpretato
	if m != nil {
		uso, ok := usi[d.BundleID]
		if !ok {
			uso = evidenze.UsoSconosciuto(d.BundleID)
		}
		it, err := m.Interpreta(d, uso)
		if err != nil {
			return MessaggioInterpretato{}, ancoraggio.EsitoProdotti{}, fmt.Errorf("valutazione: messaggio fuori RFQ %s: %w", msg.ID, err)
		}
		mi.Interpretazione = it
		interpretati = append(interpretati, ancoraggio.MessaggioInterpretato{MessaggioID: msg.ID, Documento: d, Interpretazione: it})
	}
	prodotti, err := ancoraggio.ProponiProdotti(interpretati, richiesta)
	if err != nil {
		return MessaggioInterpretato{}, ancoraggio.EsitoProdotti{}, fmt.Errorf("valutazione: messaggio fuori RFQ %s: %w", msg.ID, err)
	}
	return mi, prodotti, nil
}

// ordinaDiagnostiche: le diagnostiche in ordine di (codice, percorso, riferimenti) e poi del messaggio (chiaveDiagnostica).
func ordinaDiagnostiche(d []evidenze.Diagnostica) {
	sort.SliceStable(d, func(i, j int) bool { return chiaveDiagnostica(d[i]) < chiaveDiagnostica(d[j]) })
}
