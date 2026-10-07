// L1 — le correzioni della revisione di V1 (B6; F0-05, R34, R30 e, R31 b, F0-13, T-B6-29, IM.1): l'errore
// dell'adattatore su un file che diventa un avviso, nel thread e nel messaggio fuori RFQ (R-61); la radice che non si
// legge, un solo "" fra le radici del candidato (R-62); il messaggio fuori RFQ che nessun caso senza thread elenca, con
// il suo codice (R-63); il thread e il messaggio fuori RFQ ripetuti, errore di contratto (R-66); un caso per ognuna delle
// mutazioni vive (R-67): i motivi dei file di un thread non valutato e la disponibilità del file letto, l'equivalenza
// delle revisioni, il token sospeso nel vecchio, il caso con il thread che elenca i messaggi della sua richiesta, l'ordine
// dei conflitti composti nell'esito. R-64, R-65 e F0-19 sono in vecchio_test.go.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package valutazione_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// diagnosticheDelFile: le diagnostiche che nominano il file, nei riferimenti o nel percorso.
func diagnosticheDelFile(d []evidenze.Diagnostica, id uuid.UUID) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, x := range d {
		if contieneRif(x.Rif, id.String()) || strings.Contains(x.Percorso, id.String()) {
			out = append(out, x)
		}
	}
	return out
}

// TestR61LErroreDiUnFileDiventaUnAvviso (R-61 della revisione di V1; F0-05; 6.4.6, passo 3): un file i cui fatti hanno
// un'impronta che non è la loro fa fallire l'adattatore (estrazione.DaAllegato). Il file resta fuori, con il motivo
// documento_non_leggibile, il thread resta valutato, e l'errore diventa una diagnostica di gravità avviso, con il percorso
// file[<id>] e l'allegato nei riferimenti. Lo stesso per un file di un messaggio fuori RFQ.
func TestR61LErroreDiUnFileDiventaUnAvviso(t *testing.T) {
	controlla := func(t *testing.T, d []evidenze.Diagnostica, id uuid.UUID) {
		t.Helper()
		suo := diagnosticheDelFile(d, id)
		if len(suo) == 0 {
			t.Fatalf("l'errore dell'adattatore sul file %s non ha nessuna diagnostica", id)
		}
		for _, x := range suo {
			if x.Gravita != evidenze.GravitaAvviso || !strings.HasPrefix(x.Percorso, "file["+id.String()+"]") || !contieneRif(x.Rif, id.String()) {
				t.Errorf("diagnostica %+v: attesa un avviso con il percorso file[<id>] e l'allegato", x)
			}
		}
	}
	t.Run("il thread", func(t *testing.T) {
		th := scenaCollegamento(t)
		rotto := uid(0x6d1)
		fr := fattiPDF(t, sha("7"), "7120200A1")
		fr.Digest = strings.Repeat("9", 64)
		conAllegato(&th, allegato(rotto, 9, "rotto-acme.pdf", "pdf", sha("7")), &fr)
		et := esitoThread(t, calcola(t, fotografiaDi(th), insiemeACME(t), valutazione.Ingressi{}), threadACME)
		if !et.Valutato || fileInterpretatoDi(t, et, rotto).Motivo != valutazione.MotivoFileDocumentoNonLeggibile {
			t.Fatalf("valutato %v, motivo %q", et.Valutato, fileInterpretatoDi(t, et, rotto).Motivo)
		}
		controlla(t, et.Diagnostiche, rotto)
	})
	t.Run("il messaggio fuori RFQ", func(t *testing.T) {
		mUno, rotto := uid(0x6f1), uid(0x6f5)
		fr := fattiPDF(t, sha("5"), "7120100A2")
		fr.Digest = strings.Repeat("9", 64)
		f := fotografiaDi(scenaCollegamento(t))
		f.FuoriRFQ = []fotorfq.MessaggioFuoriRFQ{messaggioFuori(mUno, clienteACME, "Buongiorno,\r\nvi ordiniamo 7120100A2.",
			[]fotorfq.Allegato{allegato(rotto, 1, "ordine-acme.pdf", "pdf", sha("5"))}, fr)}
		in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{{ID: "censimento-acme", ClienteID: clienteACME, Messaggi: []uuid.UUID{mUno}}}}
		e := calcola(t, f, insiemeACME(t), in)
		if len(e.FuoriRFQ) != 1 || !e.FuoriRFQ[0].Valutato || e.FuoriRFQ[0].File[0].Motivo != valutazione.MotivoFileDocumentoNonLeggibile {
			t.Fatalf("fuori RFQ %+v", e.FuoriRFQ)
		}
		controlla(t, e.FuoriRFQ[0].Diagnostiche, rotto)
	})
}

// TestR61LeDiagnosticheDeiFileSullaFunzione (R-61 della revisione di V1; F0-05): sulla funzione, con errori sintetici. Un
// errore che porta un errore di contratto dà le sue diagnostiche, come avviso, con la natura d'origine, il percorso sotto
// file[<id>] e l'allegato fra i riferimenti; un errore qualunque dà valutazione.errore_valutazione, avviso, dati; in
// ordine di allegato. L'errore d'origine non cambia.
func TestR61LeDiagnosticheDeiFileSullaFunzione(t *testing.T) {
	a, b := uid(0xe41), uid(0xe42)
	ec := &evidenze.ErroreContratto{Diagnostiche: []evidenze.Diagnostica{{Codice: evidenze.CodiceDocumentoIDRipetuto, Gravita: evidenze.GravitaErrore,
		Natura: evidenze.NaturaContratto, Percorso: "unita[0]", Messaggio: "un'unità ripetuta", Rif: []string{"u:nome"}}}}
	d := valutazione.DiagnosticheDeiFilePerProva(map[uuid.UUID]error{b: errors.New("fatti di un altro contenuto"), a: ec})
	if len(d) != 2 {
		t.Fatalf("diagnostiche %+v", d)
	}
	if x := d[0]; x.Codice != evidenze.CodiceDocumentoIDRipetuto || x.Gravita != evidenze.GravitaAvviso || x.Natura != evidenze.NaturaContratto ||
		x.Percorso != "file["+a.String()+"].unita[0]" || !reflect.DeepEqual(x.Rif, []string{"u:nome", a.String()}) {
		t.Errorf("la diagnostica copiata: %+v", x)
	}
	if x := d[1]; x.Codice != valutazione.CodiceErroreValutazione || x.Gravita != evidenze.GravitaAvviso || x.Natura != evidenze.NaturaDati ||
		x.Percorso != "file["+b.String()+"]" || !reflect.DeepEqual(x.Rif, []string{b.String()}) || !strings.Contains(x.Messaggio, "fatti di un altro contenuto") {
		t.Errorf("l'errore qualunque: %+v", x)
	}
	if o := ec.Diagnostiche[0]; o.Gravita != evidenze.GravitaErrore || o.Percorso != "unita[0]" || len(o.Rif) != 1 {
		t.Errorf("l'errore d'origine è cambiato: %+v", o)
	}
}

// scenaRadiceIllegibile: la scena dell'albero (il prodotto 7120100A con la fonte confermata e lo sciolto 7120200A) e un
// secondo finito, con un codice che la grammatica non legge, e con il suo STEP autorizzato (il gesto 3, la vista, la
// marcatura della radice): la sua BOM di lavoro ha anche lo sciolto. Un 2D con il codice dello sciolto si raggiunge da
// tutti e due i prodotti.
func scenaRadiceIllegibile(t *testing.T) (fotorfq.Thread, uuid.UUID) {
	t.Helper()
	th := scenaAlbero(t)
	cIll, aSt2, dSt2, f2D := uid(0xe01), uid(0xe02), uid(0xe03), uid(0xe21)
	th.Componenti = append(th.Componenti, componente(cIll, "ASSIEME-ACME", "finito", "manuale"))
	f := fattiStepF(t, shaN(301), []nodoF{{"#1", "ASSIEME-ACME", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 1}})
	conAllegato(&th, allegato(aSt2, 30, "assieme-acme.stp", "stp", shaN(301)), &f)
	ci, dd, op := cIll, dSt2, operatore
	th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: dSt2, ThreadID: threadACME, ComponenteID: &ci, Tipo: "cad_3d", NomeFile: "assieme-acme.stp",
		Estensione: "stp", Sha256: shaN(301), StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME, Allegati: []uuid.UUID{aSt2}})
	th.Componenti[len(th.Componenti)-1].StepStrutturaleID = &dd
	th.StepProdotto = append(th.StepProdotto, fotorfq.RigaStepProdotto{ComponenteID: cIll, Codice: "ASSIEME-ACME", StepStrutturaleID: &dd, NStepCorrenti: 1,
		AnalisiCompleta: true, Esito: "presente_analizzato"})
	th.RigheComponenteProposta = append(th.RigheComponenteProposta,
		fotorfq.RigaComponenteProposta{ID: uid(0xe11), AllegatoID: aSt2, NomeFile: "assieme-acme.stp", Sha256: shaN(301), Chiave: "#1", IDGrezzo: "ASSIEME-ACME",
			Famiglia: "acme-catena", Fonte: "step", Stato: "aperta", Marcatura: &fotorfq.MarcaturaStrutturale{Versione: 1, Ruolo: "radice", ComponenteID: &ci,
				DocumentoID: &dd, DichiaratoDa: &op, DichiaratoIl: "2026-10-01T08:00:00Z"}},
		fotorfq.RigaComponenteProposta{ID: uid(0xe12), AllegatoID: aSt2, NomeFile: "assieme-acme.stp", Sha256: shaN(301), Chiave: "#2", IDGrezzo: "7120200A",
			Famiglia: "acme-catena", Fonte: "step", Stato: "aperta"})
	fileNelMessaggio(&th, f2D, idM1, "7120200A_1.pdf", "pdf", shaN(302), scansione(t, shaN(302)))
	return th, f2D
}

// TestR62UnaRadiceCheNonSiLegge (R-62 della revisione di V1; R30 e; R-41 della revisione di P8): il 2D dello sciolto si
// raggiunge dal prodotto 7120100A e dal finito con il codice che non si legge (la base del target vuota, la BOM di lavoro
// dalla fonte confermata). Fra le radici del candidato piatto la radice che non si legge resta, come un solo "": una
// radice in più non sparisce, e confronto la conta come tale.
func TestR62UnaRadiceCheNonSiLegge(t *testing.T) {
	th, f2D := scenaRadiceIllegibile(t)
	vistaCome(&th)
	et := esitoThread(t, calcola(t, fotografia(th), insiemeACME(t), valutazione.Ingressi{}), threadACME)
	if p := prodottoDi(t, et, "componente:"+uid(0xe01).String()); p.Base.Normalizzata != "" || p.Fonte.Stato != valutazione.FonteConfermata {
		t.Fatalf("il finito con il codice che non si legge: base %q, fonte %s", p.Base.Normalizzata, p.Fonte.Stato)
	}
	n := confrontabileDi(t, et, f2D).Nuovo
	if len(n.Candidati) != 1 || !reflect.DeepEqual(n.Candidati[0].Radici, []string{"", "7120100"}) {
		t.Errorf("candidati %+v: la radice che non si legge è un solo \"\" fra le radici", n.Candidati)
	}
}

// TestR63IlMessaggioCheNessunCasoSenzaThreadElenca (R-63 della revisione di V1; T-B6-26, R34): un messaggio caricato fra i
// fuori RFQ, che elenca solo un caso con il thread (i messaggi della sua richiesta), non si valuta: un avviso con il codice
// suo, mai valutazione.errore_valutazione, che resta per l'errore vero.
func TestR63IlMessaggioCheNessunCasoSenzaThreadElenca(t *testing.T) {
	f := fotografiaDi(scenaCollegamento(t))
	mX := uid(0x6f9)
	f.FuoriRFQ = []fotorfq.MessaggioFuoriRFQ{messaggioFuori(mX, clienteACME, "Buongiorno,\r\nvi ordiniamo 7120100A2.", nil)}
	th := threadACME
	in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{{ID: "caso-acme", ThreadID: &th, ClienteID: clienteACME, Messaggi: []uuid.UUID{mX}}}}
	e := calcola(t, f, insiemeACME(t), in)
	d := conCodice(e.Diagnostiche, valutazione.CodiceMessaggioFuoriRFQSenzaCaso)
	if len(d) != 1 || d[0].Gravita != evidenze.GravitaAvviso || d[0].Natura != evidenze.NaturaDati || !reflect.DeepEqual(d[0].Rif, []string{mX.String()}) ||
		contieneCodice(e.Diagnostiche, valutazione.CodiceErroreValutazione) || len(e.FuoriRFQ) != 0 {
		t.Errorf("diagnostiche %+v, fuori RFQ %d", e.Diagnostiche, len(e.FuoriRFQ))
	}
}

// TestR66IlThreadEIlMessaggioRipetuti (R-66 della revisione di V1; IM.1): un thread con lo stesso ID due volte nella
// fotografia, o un messaggio fuori RFQ ripetuto, è un errore di contratto (contratto.id_ripetuto), con un esito vuoto:
// altrimenti l'esito dipenderebbe dall'ordine.
func TestR66IlThreadEIlMessaggioRipetuti(t *testing.T) {
	r := insiemeACME(t)
	controlla := func(t *testing.T, f fotorfq.Fotografia, percorso string) {
		t.Helper()
		e, err := valutazione.Calcola(f, r, valutazione.Ingressi{})
		var ec *evidenze.ErroreContratto
		if !errors.As(err, &ec) || e.Impronta != "" || len(e.Thread) != 0 {
			t.Fatalf("errore %v, esito con %d thread", err, len(e.Thread))
		}
		for _, d := range ec.Diagnostiche {
			if d.Codice == grammatica.CodiceIDRipetuto && d.Percorso == percorso && d.Gravita == evidenze.GravitaErrore && d.Natura == evidenze.NaturaContratto {
				return
			}
		}
		t.Errorf("nessun %s su %s fra %+v", grammatica.CodiceIDRipetuto, percorso, ec.Diagnostiche)
	}
	t.Run("il thread", func(t *testing.T) {
		a := threadDi(threadSenzaRegole, clienteSenzaRegole, mSenzaRegole, "Buongiorno,\r\nvi chiediamo 7120100A2.")
		b := threadDi(threadSenzaRegole, clienteSenzaRegole, mSenzaRegole, "Buongiorno,\r\nvi chiediamo 7120200A1.")
		controlla(t, fotografiaDi(scenaCollegamento(t), a, b), "thread["+threadSenzaRegole.String()+"]")
	})
	t.Run("il messaggio fuori RFQ", func(t *testing.T) {
		mUno := uid(0x6f1)
		f := fotografiaDi(scenaCollegamento(t))
		mf := messaggioFuori(mUno, clienteACME, "Buongiorno,\r\nvi ordiniamo 7120100A2.", nil)
		f.FuoriRFQ = []fotorfq.MessaggioFuoriRFQ{mf, mf}
		controlla(t, f, "fuori_rfq["+mUno.String()+"]")
	})
}

// TestR67LeMutazioniVive (R-67 della revisione di V1): un caso per ognuna delle mutazioni vive non equivalenti.
func TestR67LeMutazioniVive(t *testing.T) {
	// M09 e M11 (T-B6-29): in un thread non valutato (il cliente senza voce nell'indice) il contenitore e l'inline hanno il
	// loro motivo, che viene prima di thread_non_valutato; il file letto (la voce dello zip, una scansione) porta la sua
	// disponibilità anche se non è valutato.
	t.Run("i file di un thread non valutato", func(t *testing.T) {
		th := threadDi(threadSenzaRegole, clienteSenzaRegole, mSenzaRegole, "Buongiorno,\r\nvi chiediamo 7120100A2.")
		zip, voce, inline := uid(0xe31), uid(0xe32), uid(0xe33)
		z := allegato(zip, 1, "pacco-acme.zip", "zip", shaN(311))
		v := allegato(voce, 2, "voce-acme.pdf", "pdf", shaN(312))
		i := allegato(inline, 3, "logo.png", "png", shaN(313))
		z.MessaggioID, v.MessaggioID, i.MessaggioID, v.ContenitoreID, i.Natura = mSenzaRegole, mSenzaRegole, mSenzaRegole, &zip, "inline"
		conAllegato(&th, z, nil)
		conAllegato(&th, v, scansione(t, shaN(312)))
		conAllegato(&th, i, nil)
		et := esitoThread(t, calcola(t, fotografiaDi(scenaCollegamento(t), th), insiemeACME(t), valutazione.Ingressi{}), threadSenzaRegole)
		if et.Valutato {
			t.Fatal("il thread del cliente senza regole è valutato")
		}
		for _, c := range []struct {
			id                    uuid.UUID
			motivo, disponibilita string
		}{{zip, valutazione.MotivoFileContenitore, ""}, {inline, valutazione.MotivoFileNaturaNonFile, ""},
			{voce, valutazione.MotivoFileThreadNonValutato, "senza_testo"}} {
			if n := confrontabileDi(t, et, c.id).Nuovo; n.Valutato || n.Motivo != c.motivo || n.Disponibilita != c.disponibilita {
				t.Errorf("file %s: %+v, attesi il motivo %q e la disponibilità %q", c.id, n, c.motivo, c.disponibilita)
			}
		}
	})
	// M10 (R31 b; 6.4.6 passo 10): con un'equivalenza dichiarata dalla regola della revisione, il codice vecchio con la
	// revisione 01 e il cartiglio con la 1 hanno le revisioni uguali, con il motivo equivalente.
	t.Run("l'equivalenza delle revisioni", func(t *testing.T) {
		f := famigliaCatena()
		f.Revisioni[0].Segmenti[0].Pattern = "[0-9]{1,2}"
		f.Revisioni[0].Equivalenze = [][2]string{{"1", "01"}}
		r := insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", f)})
		e := calcola(t, fotografiaDi(scenaVecchio(t, "7120200A01", `{}`)), r, valutazione.Ingressi{})
		c := confrontabileDi(t, esitoThread(t, e, threadACME), aDisegno)
		if c.Vecchio.Revisione != "01" || c.Nuovo.Revisione != "1" || c.Nuovo.Revisioni != valutazione.RevisioniUguali ||
			c.Nuovo.MotivoRevisioni != valutazione.MotivoRevisioniEquivalente {
			t.Errorf("vecchio %+v, nuovo %+v", c.Vecchio, c.Nuovo)
		}
	})
	// M12 (6.4.6, LetturaRegistrata): un codice con un token sospeso al posto della revisione si legge, ma la revisione non
	// è letta, e resta vuota (la mutazione è equivalente con motorea di oggi, che dà Normalizzata vuota fuori dallo stato
	// «letta»: la prova fissa il contratto).
	t.Run("il token sospeso nel vecchio", func(t *testing.T) {
		f := famigliaCatena()
		f.Revisioni[0].TokenSospesi = []grammatica.TokenSospeso{{ID: "xx", Pattern: "xx", Riserva: "Q-ACME-1"}}
		m := motoreConFamiglia(t, f)
		if l := valutazione.LeggiCodiceRegistrato(m, "7120200Axx"); !l.Leggibile || l.Base != "7120200" || l.Revisione != "" {
			t.Errorf("il token sospeso: %+v", l)
		}
		if l := valutazione.LeggiCodiceRegistrato(m, "7120200A2"); l.Revisione != "2" {
			t.Errorf("la revisione letta: %+v", l)
		}
	})
	// M20 (R34): un caso con il thread che elenca i messaggi della sua richiesta non è un caso di censimento: nessun
	// messaggio fuori RFQ, nessuna diagnostica.
	t.Run("il caso con il thread che elenca i suoi messaggi", func(t *testing.T) {
		th := threadACME
		in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{{ID: "caso-acme", ThreadID: &th, ClienteID: clienteACME,
			Messaggi: []uuid.UUID{idM1}}}}
		e := calcola(t, fotografiaDi(scenaCollegamento(t)), insiemeACME(t), in)
		if len(e.FuoriRFQ) != 0 || len(e.Diagnostiche) != 0 || !esitoThread(t, e, threadACME).Valutato {
			t.Errorf("fuori RFQ %+v, diagnostiche %+v", e.FuoriRFQ, e.Diagnostiche)
		}
	})
	// M26 (T-B6-10, F0-13): lo stesso conflitto di gerarchia (una rimozione aperta sotto lo sciolto) su due prodotti,
	// nell'esito di Calcola: due voci, in ordine canonico e poi di prodotto, anche se i pezzi arrivano nell'ordine dei target
	// (per codice: prima il finito 7120050A, il cui Rif viene dopo).
	t.Run("l'ordine dei conflitti composti nell'esito", func(t *testing.T) {
		th := scenaSmistamento(t)
		th.Componenti = append(th.Componenti, componente(cP2, "7120050A", "finito", "manuale"))
		th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cP2, FiglioID: cSciolto, Qta: 1, Origine: "step", ConfermatoDa: operatore, CreatoIl: dataACME})
		th.RimozioniAperte = []fotorfq.RimozioneAperta{{StepDocumentoID: dStepB, PadreID: cSciolto, FiglioID: cTerzo, QtaWorking: 1}}
		et := esitoSmistamento(t, th)
		if len(et.ProdottiValutati) != 2 || et.ProdottiValutati[0].Rif != rifP2 {
			t.Fatalf("%d target, il primo %s: attesi due, prima il finito 7120050A", len(et.ProdottiValutati), et.ProdottiValutati[0].Rif)
		}
		var prodotti []string
		for _, c := range et.Conflitti {
			if c.Tipo == valutazione.ConflittoArco {
				prodotti = append(prodotti, c.Prodotto)
			}
		}
		if !reflect.DeepEqual(prodotti, []string{rifProdB, rifP2}) || canonicoDi(t, et.Conflitti) != canonicoDi(t, valutazione.ComponiConflittiPerProva(et.Conflitti)) {
			t.Errorf("i prodotti dei conflitti di gerarchia, nell'ordine: %v", prodotti)
		}
	})
}
