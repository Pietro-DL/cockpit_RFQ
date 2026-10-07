// L1 — le correzioni dopo le risposte dell'utente a R106–R117 del 07/10 (B6, Q7c; domande-a1c.md): il contesto del
// messaggio per ogni file che conta, con la provenienza (AssociazioneFile.Contesto: R106 B e R107, precisate), e la
// contraddizione visibile (valutazione.contesto_discorde): il file con un candidato nel secondo prodotto, in un
// messaggio che nomina solo il primo; il file terminale con il documento sul secondo prodotto, anche con un candidato nel
// primo (T-B6-172); il messaggio che nomina tutti e due, con l'evidenza in uno; il messaggio che nomina due prodotti,
// nessuno dei quali collegato al file (tre target); le caratteristiche generiche del file, che non sono un collegamento;
// la provenienza per la voce di un archivio, con le sole letture che nominano un target, e la storia solo con il caso;
// il marcatore della lettura del vecchio motore anche quando non c'è (il gemello CodiceLettoMarcatore di R114,
// precisata). Le prove di T-B6-172 e dei tre target vengono dalla revisione delle correzioni (R-133: RV2 e RV1).
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// Gli ID in più delle prove delle risposte.
var (
	mConAltro = uid(0xa0b) // nomina il prodotto e un codice che non è di nessun target
	zipR107   = uid(0x1063)
	cP3       = uid(0x1064) // il terzo finito manuale, 7120800A, senza STEP
	rifP3     = "componente:" + uid(0x1064).String()
	mTre      = uid(0x1065) // nomina il primo e il terzo prodotto
)

// discordiDi: le diagnostiche valutazione.contesto_discorde dell'esito per l'allegato (il primo riferimento).
func discordiDi(et valutazione.EsitoThread, allegato uuid.UUID) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, d := range conCodice(et.Diagnostiche, valutazione.CodiceContestoDiscorde) {
		if len(d.Rif) > 0 && d.Rif[0] == allegato.String() {
			out = append(out, d)
		}
	}
	return out
}

// contestoDi: i prodotti nominati del contesto dell'associazione; nil senza contesto.
func contestoDi(a valutazione.AssociazioneFile) []string {
	if a.Contesto == nil {
		return nil
	}
	return a.Contesto.Prodotti
}

// TestR106LaContraddizioneVisibile (R106 B, precisata il 07/10: «Il file ha già evidenze o decisioni che lo collegano a
// un prodotto: il contesto non deve aggiungere indiscriminatamente altre destinazioni. Una contraddizione deve restare
// visibile»): nella scena del contesto (PO-40) il file W ha un candidato nel secondo prodotto ed è allegato al messaggio
// che nomina solo il primo. Il contesto c'è, con la provenienza; la pertinenza resta quella dell'evidenza; la
// diagnostica valutazione.contesto_discorde lo dice, con l'allegato, il prodotto nominato e quello dell'evidenza. Non
// blocca niente: con W allegato al messaggio che nomina tutti e due (nessuna contraddizione) i prodotti, le loro impronte
// e il fascicolo sono gli stessi.
func TestR106LaContraddizioneVisibile(t *testing.T) {
	et := esitoSmistamento(t, scenaContesto(t))
	w := associazioneDi(t, et, fW)
	if w.Contesto == nil || w.Contesto.MessaggioID != idM1 || !reflect.DeepEqual(w.Contesto.Prodotti, []string{rifProdB}) || len(w.Contesto.Letture) == 0 {
		t.Fatalf("il contesto di W: %+v", w.Contesto)
	}
	if !reflect.DeepEqual(w.Pertinente, []string{rifP2}) || w.PertinenzaContesto != nil {
		t.Errorf("W: pertinente %v, contesto nella pertinenza %v: il contesto non aggiunge destinazioni", w.Pertinente, w.PertinenzaContesto)
	}
	d := discordiDi(et, fW)
	if len(d) != 1 || d[0].Gravita != evidenze.GravitaAvviso || d[0].Natura != evidenze.NaturaDati ||
		d[0].Percorso != "associazioni["+fW.String()+"].contesto" ||
		!reflect.DeepEqual(d[0].Rif, []string{fW.String(), "contesto:" + rifProdB, "evidenza:" + rifP2}) {
		t.Fatalf("la contraddizione di W: %+v", d)
	}
	p1 := prodottoDi(t, et, rifProdB)
	if nonTerminale(p1.Smistamento, fW) {
		t.Errorf("il primo prodotto riceve W: %+v", p1.Smistamento)
	}

	// Lo stesso W nel messaggio che nomina tutti e due: nessuna contraddizione, e niente altro cambia.
	th := scenaContesto(t)
	for i := range th.Allegati {
		if th.Allegati[i].ID == fW {
			th.Allegati[i].MessaggioID = mDue
		}
	}
	senza := esitoSmistamento(t, th)
	if len(discordiDi(senza, fW)) != 0 || !reflect.DeepEqual(contestoDi(associazioneDi(t, senza, fW)), []string{rifProdB, rifP2}) {
		t.Errorf("W nel messaggio che nomina tutti e due: contraddizione %+v, contesto %+v", discordiDi(senza, fW), associazioneDi(t, senza, fW).Contesto)
	}
	for _, rif := range []string{rifProdB, rifP2} {
		if canonicoDi(t, prodottoDi(t, et, rif)) != canonicoDi(t, prodottoDi(t, senza, rif)) {
			t.Errorf("il prodotto %s cambia con la contraddizione: non deve bloccare né entrare nell'impronta", rif)
		}
	}
	if canonicoDi(t, et.Fascicolo) != canonicoDi(t, senza.Fascicolo) {
		t.Error("il fascicolo cambia con la contraddizione")
	}
	a, b := associazioneDi(t, et, fW), associazioneDi(t, senza, fW)
	a.Contesto, b.Contesto = nil, nil
	if !reflect.DeepEqual(a, b) {
		t.Errorf("l'associazione di W cambia oltre il contesto:\n%+v\n%+v", a, b)
	}

	// Con una sezione dello smistamento assente (T-12) la pertinenza non si calcola: il contesto resta, con la
	// provenienza, ma la contraddizione non si dichiara, perché le evidenze possono mancare (dubbio T-B6-171).
	t.Run("T-12", func(t *testing.T) {
		th := scenaContesto(t)
		vistaCome(&th)
		f := fotografia(th)
		f.Sezioni[fotorfq.SezioneProposteDocumento] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
		et := esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME)
		discordi := conCodice(et.Diagnostiche, valutazione.CodiceContestoDiscorde)
		if w := associazioneDi(t, et, fW); !reflect.DeepEqual(contestoDi(w), []string{rifProdB}) || len(discordi) != 0 {
			t.Errorf("con T-12: contesto %+v, contraddizioni %+v", w.Contesto, discordi)
		}
	})
}

// TestR106IlFileTerminale (R106 B, precisata: «Le decisioni già registrate non vengono sostituite dal ricalcolo»): un
// documento confermato sul componente del secondo prodotto porta un file allegato al messaggio che nomina solo il primo.
// Il file è terminale, la decisione resta, il primo prodotto non lo riceve; il contesto c'è, e la contraddizione con la
// decisione (i prodotti del componente del documento) si vede.
func TestR106IlFileTerminale(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th)
	pdfSenzaCodice(t, &th, fD, idM1, "disegno-secondo.pdf", 201)
	doc := uid(0x1061)
	th.Documenti = append(th.Documenti, documento2D(doc, cP2, fD, "disegno-secondo.pdf", "pdf", shaN(201)))
	et := esitoSmistamento(t, th)
	a := associazioneDi(t, et, fD)
	if !a.Terminale || a.Confermata == nil || *a.Confermata != cP2 || a.DocumentoID == nil || *a.DocumentoID != doc || a.Origine != ancoraggio.OrigineConfermato {
		t.Fatalf("la decisione resta: %+v", a)
	}
	if !reflect.DeepEqual(a.Pertinente, []string{rifP2}) || a.PertinenzaContesto != nil || !reflect.DeepEqual(contestoDi(a), []string{rifProdB}) {
		t.Errorf("pertinente %v, contesto nella pertinenza %v, contesto %+v", a.Pertinente, a.PertinenzaContesto, a.Contesto)
	}
	d := discordiDi(et, fD)
	if len(d) != 1 || !reflect.DeepEqual(d[0].Rif, []string{fD.String(), "contesto:" + rifProdB, "decisione:" + rifP2}) {
		t.Errorf("la contraddizione con la decisione: %+v", d)
	}
	if p1 := prodottoDi(t, et, rifProdB); nonTerminale(p1.Smistamento, fD) || p1.Smistamento.Stato != valutazione.SmistamentoVerificato {
		t.Errorf("il primo prodotto riceve il file: %+v", p1.Smistamento)
	}
	if _, ok := daSmistareDi(et, fD); ok {
		t.Error("un file terminale è da smistare")
	}
}

// TestR106IlFileTerminaleConUnCandidatoDiscorde (R106 B, precisata; T-B6-172, fissato dalla revisione delle correzioni:
// R-133, RV2): il file ha nel nome il codice dello sciolto, un componente del primo prodotto (un candidato del motore A
// nel primo), e un documento confermato sul componente del secondo; il messaggio nomina solo il primo. Il file è
// terminale e si confronta con la sola decisione (il secondo), non con le evidenze (che comprendono il primo): la
// contraddizione si vede, con il Rif della decisione.
func TestR106IlFileTerminaleConUnCandidatoDiscorde(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th)
	fileNelMessaggio(&th, fD, idM1, "7120200A_1.pdf", "pdf", shaN(231), scansione(t, shaN(231)))
	th.Documenti = append(th.Documenti, documento2D(uid(0x1066), cP2, fD, "7120200A_1.pdf", "pdf", shaN(231)))
	et := esitoSmistamento(t, th)
	a := associazioneDi(t, et, fD)
	if !a.Terminale || a.Confermata == nil || *a.Confermata != cP2 || !reflect.DeepEqual(contestoDi(a), []string{rifProdB}) {
		t.Fatalf("il file: %+v", a)
	}
	if !contieneRif(a.Pertinente, rifProdB) || !contieneRif(a.Pertinente, rifP2) {
		t.Fatalf("le evidenze del file comprendono il primo prodotto (il candidato) e il secondo (il documento): %v", a.Pertinente)
	}
	d := discordiDi(et, fD)
	if len(d) != 1 || !reflect.DeepEqual(d[0].Rif, []string{fD.String(), "contesto:" + rifProdB, "decisione:" + rifP2}) {
		t.Errorf("la contraddizione con la decisione, con un candidato nel prodotto nominato: %+v", d)
	}
}

// TestR106PiuProdottiNessunoCollegato (R106 B, precisata: «Il messaggio coinvolge più prodotti e non distingue
// l'allegato»; fissata dalla revisione delle correzioni: R-133, RV1): con tre target, un file con un candidato nel
// secondo prodotto è allegato al messaggio che nomina il primo e il terzo. Il contesto conserva i due nominati; il file
// resta pertinente al solo secondo, non blocca né il primo né il terzo, e non è orfano. Nessuno dei nominati è
// collegato al file: la contraddizione si vede anche con più prodotti nominati.
func TestR106PiuProdottiNessunoCollegato(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th)
	th.Componenti = append(th.Componenti, componente(cP3, "7120800A", "finito", "manuale"))
	th.StepProdotto = append(th.StepProdotto, fotorfq.RigaStepProdotto{ComponenteID: cP3, Codice: "7120800A", Esito: "da_scegliere"})
	conMessaggio(&th, mTre, 40, "Altri file", "Buongiorno,\r\nper 7120100A e 7120800A vi mandiamo i file.\r\nGrazie", true)
	fileNelMessaggio(&th, fG, mTre, "7120900A_2.pdf", "pdf", shaN(232), scansione(t, shaN(232)))
	et := esitoSmistamento(t, th)
	prodottoDi(t, et, rifP3) // il terzo è un target
	g := associazioneDi(t, et, fG)
	if g.Contesto == nil || g.Contesto.MessaggioID != mTre || !reflect.DeepEqual(g.Contesto.Prodotti, []string{rifProdB, rifP3}) {
		t.Fatalf("il contesto di G: %+v", g.Contesto)
	}
	if !reflect.DeepEqual(g.Pertinente, []string{rifP2}) || g.PertinenzaContesto != nil {
		t.Errorf("G: pertinente %v, contesto nella pertinenza %v", g.Pertinente, g.PertinenzaContesto)
	}
	for _, rif := range []string{rifProdB, rifP3} {
		if nonTerminale(prodottoDi(t, et, rif).Smistamento, fG) {
			t.Errorf("G blocca il prodotto nominato %s", rif)
		}
	}
	if d, ok := daSmistareDi(et, fG); ok && (d.Orfano || d.ProdottiContesto != nil) {
		t.Errorf("G da smistare: %+v", d)
	}
	d := discordiDi(et, fG)
	if len(d) != 1 || !reflect.DeepEqual(d[0].Rif, []string{fG.String(), "contesto:" + rifProdB, "contesto:" + rifP3, "evidenza:" + rifP2}) {
		t.Errorf("la contraddizione con due prodotti nominati: %+v", d)
	}
}

// TestR106PiuProdottiConUnEvidenza (R106 B, precisata: «Il messaggio coinvolge più prodotti e non distingue l'allegato:
// conserva i riferimenti possibili, senza rendere automaticamente il file pertinente a tutti e senza bloccare tutti
// indistintamente»): un file con un candidato nel secondo prodotto, allegato al messaggio che nomina tutti e due. Il
// contesto conserva i due prodotti anche se il file non è orfano; il file resta pertinente al solo secondo; nessuna
// contraddizione, perché il secondo è fra i nominati.
func TestR106PiuProdottiConUnEvidenza(t *testing.T) {
	th := scenaContesto(t)
	fileNelMessaggio(&th, fG, mDue, "7120900A_2.pdf", "pdf", shaN(202), scansione(t, shaN(202)))
	et := esitoSmistamento(t, th)
	g := associazioneDi(t, et, fG)
	if g.Contesto == nil || g.Contesto.MessaggioID != mDue || !reflect.DeepEqual(g.Contesto.Prodotti, []string{rifProdB, rifP2}) {
		t.Fatalf("il contesto di G: %+v", g.Contesto)
	}
	if !reflect.DeepEqual(g.Pertinente, []string{rifP2}) || g.PertinenzaContesto != nil || len(discordiDi(et, fG)) != 0 {
		t.Errorf("G: pertinente %v, contesto nella pertinenza %v, contraddizione %+v", g.Pertinente, g.PertinenzaContesto, discordiDi(et, fG))
	}
	if nonTerminale(prodottoDi(t, et, rifProdB).Smistamento, fG) || !nonTerminale(prodottoDi(t, et, rifP2).Smistamento, fG) {
		t.Errorf("G blocca il primo prodotto, o non il secondo")
	}
	if d, ok := daSmistareDi(et, fG); ok && (d.Orfano || d.ProdottiContesto != nil) {
		t.Errorf("G da smistare: %+v (i prodotti del contesto restano solo per l'orfano)", d)
	}
}

// TestR106LeCaratteristicheGeneriche (R106 B, precisata: «Una caratteristica generica del file, come l'estensione PDF,
// non è un collegamento a un prodotto»; fissa il comportamento di oggi): un PDF senza codice, con la proposta disegno_2d
// e la destinazione «generale», allegato al messaggio che nomina solo il prodotto. Né il tipo proposto né la destinazione
// generale sono evidenze: il file è pertinente al prodotto per il contesto, e non c'è nessuna contraddizione.
func TestR106LeCaratteristicheGeneriche(t *testing.T) {
	th := scenaSmistamento(t)
	pdfSenzaCodice(t, &th, fX, idM1, "allegato-uno.pdf", 203)
	conProposta(&th, uid(0x1062), fX, "disegno_2d", "nome_file", "aperta", nil, nil).Dettagli = destinazione(t, "generale")
	et := esitoSmistamento(t, th)
	x := associazioneDi(t, et, fX)
	if !reflect.DeepEqual(x.PertinenzaContesto, []string{rifProdB}) || !reflect.DeepEqual(x.Pertinente, []string{rifProdB}) ||
		!reflect.DeepEqual(x.DestinazioneF8, []string{"generale"}) || !reflect.DeepEqual(contestoDi(x), []string{rifProdB}) {
		t.Errorf("X: %+v", x)
	}
	if len(discordiDi(et, fX)) != 0 || !nonTerminale(prodottoDi(t, et, rifProdB).Smistamento, fX) {
		t.Errorf("X: contraddizione %+v, oppure non tiene aperto il prodotto", discordiDi(et, fX))
	}
}

// TestR107LaProvenienza (R107, precisata il 07/10: «preservando sempre il messaggio e il segmento da cui deriva
// l'informazione»; in A1c la formula dell'opzione A, lettura [T]): il contesto porta il messaggio dell'allegato esterno,
// anche per la voce di un archivio allegato a un altro messaggio, e gli ID delle sole letture che nominano un target,
// dell'oggetto o del corpo del messaggio (un codice che non è di nessun target non entra). La storia di un inoltro senza
// confine non dà contesto senza un caso; con il caso che la dichiara pertinente le letture sono della storia.
func TestR107LaProvenienza(t *testing.T) {
	r := insiemeConStoria(t)
	scena := func() fotorfq.Thread {
		th := scenaSmistamento(t)
		secondoProdotto(t, &th)
		conMessaggio(&th, mNessuno, 30, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		conMessaggio(&th, mConAltro, 35, "Altri file", "Buongiorno,\r\nper 7120100A e per il particolare 7120555A i file.\r\nGrazie", true)
		conMessaggio(&th, mInoltro, 20, "I: Richiesta ACME26-031", "Buongiorno,\r\nvi giro la richiesta per 7120900A.\r\nGrazie", true)
		conAllegato(&th, allegato(zipR107, 60, "pacco.zip", "zip", shaN(211)), nil)
		v := allegato(fU, 61, "voce.pdf", "pdf", shaN(212))
		v.MessaggioID, v.ContenitoreID = mNessuno, &zipR107
		conAllegato(&th, v, scansione(t, shaN(212)))
		pdfSenzaCodice(t, &th, fX, mConAltro, "allegato-altro.pdf", 213)
		pdfSenzaCodice(t, &th, fZ, mInoltro, "allegato-inoltro.pdf", 214)
		return th
	}
	dellaPosizione := func(c *valutazione.ContestoMessaggio, prefissi ...string) bool {
		if c == nil || len(c.Letture) == 0 {
			return false
		}
		for _, l := range c.Letture {
			ok := false
			for _, p := range prefissi {
				ok = ok || strings.HasPrefix(l, p)
			}
			if !ok {
				return false
			}
		}
		return true
	}
	et := esitoSmistamentoCon(t, scena(), r)
	u := associazioneDi(t, et, fU)
	if u.Contesto == nil || u.Contesto.MessaggioID != idM1 || !reflect.DeepEqual(u.Contesto.Prodotti, []string{rifProdB}) ||
		!dellaPosizione(u.Contesto, "l:u:oggetto:", "l:u:corpo:") {
		t.Errorf("la voce dell'archivio: il messaggio dell'allegato esterno %s, %+v", idM1, u.Contesto)
	}
	x := associazioneDi(t, et, fX)
	if x.Contesto == nil || x.Contesto.MessaggioID != mConAltro || !reflect.DeepEqual(x.Contesto.Prodotti, []string{rifProdB}) ||
		len(x.Contesto.Letture) != 1 || !dellaPosizione(x.Contesto, "l:u:corpo:") {
		t.Errorf("solo la lettura che nomina un target: %+v", x.Contesto)
	}
	if z := associazioneDi(t, et, fZ); z.Contesto != nil {
		t.Errorf("la storia senza il caso dà il contesto: %+v", z.Contesto)
	}

	tid := threadACME
	caso := valutazione.IngressoCaso{ID: "ACME-STORIA", ThreadID: &tid, ClienteID: clienteACME,
		Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: mInoltro, SegmentoID: "s:storia:1", Uso: "pertinente", Origine: "scenario"}}}
	et = esitoSmistamentoCon(t, scena(), r, caso)
	if z := associazioneDi(t, et, fZ); z.Contesto == nil || z.Contesto.MessaggioID != mInoltro ||
		!reflect.DeepEqual(z.Contesto.Prodotti, []string{rifP2}) || !dellaPosizione(z.Contesto, "l:u:storia:") {
		t.Errorf("la storia dichiarata dal caso: %+v", z.Contesto)
	}
}

// TestR114IlMarcatoreCheNonCe (R114, precisata il 07/10; il gemello CodiceLettoMarcatore): con una famiglia in cui il
// marcatore è facoltativo, la lettura del vecchio motore senza marcatore si legge (la base c'è) e il marcatore resta
// vuoto, anche se il codice registrato del documento ne ha uno: il marcatore non si inventa e non si prende altrove.
func TestR114IlMarcatoreCheNonCe(t *testing.T) {
	f := famigliaCatena()
	for i := range f.Forme {
		for j := range f.Forme[i].Parti {
			if f.Forme[i].Parti[j].Tipo == grammatica.TipoParteMarcatore {
				f.Forme[i].Parti[j].Min = 0
			}
		}
	}
	m := motoreConFamiglia(t, f)
	if l := valutazione.LeggiCodiceRegistrato(m, "7120299"); !l.Leggibile || l.Base != "7120299" || l.Marcatore != "" {
		t.Fatalf("il codice senza marcatore non si legge come atteso: %+v", l)
	}
	r := insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", f)})
	dettagli := `{"valutazione": {"v": 2, "codice": {"valore": "7120299", "regola": "cartiglio"}}}`
	e := calcola(t, fotografiaDi(scenaVecchio(t, "7120200A1", dettagli)), r, valutazione.Ingressi{})
	v := confrontabileDi(t, esitoThread(t, e, threadACME), aDisegno).Vecchio
	if v.CodiceLetto != "7120299" || v.CodiceLettoBase != "7120299" || v.CodiceLettoMarcatore != "" || v.Marcatore != "A" {
		t.Errorf("vecchio: codice letto %q, base %q, marcatore %q (quello del documento: %q)", v.CodiceLetto, v.CodiceLettoBase, v.CodiceLettoMarcatore, v.Marcatore)
	}
}
