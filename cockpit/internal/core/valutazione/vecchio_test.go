// L1 — il vecchio di ogni file in valutazione (A1c-L1-30; piano A, 6.4.6, LetturaRegistrata, LeggiCodiceRegistrato,
// VecchioLetto, passo 10; R31 c; registro §10.2; T-B0-14, T-B1-07, T-B6-06; F0-02, F0-10, F0-18): il codice registrato
// letto con la grammatica (con la A, con la B, con la X che non si legge, con una maiuscola diversa: mai un confronto per
// stringa), con confronto.codice_registrato_non_leggibile; il componente vecchio dal documento, non dalla proposta; la
// lettura del vecchio motore e la destinazione F8 dai dettagli, con la base (F0-19) e il marcatore della lettura (il
// gemello di R114, precisata dall'utente il 07/10); le revisioni confrontate con la grammatica (uguali, diverse, non
// confrontabili con ogni motivo); il vecchio dei prodotti.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package valutazione_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// TestL130LaLetturaDelCodiceRegistrato (A1c-L1-30; R31 c; T-B1-07): l'involucro esportato della regola di B1, con il
// motivo; la stessa lettura dei target.
func TestL130LaLetturaDelCodiceRegistrato(t *testing.T) {
	m := motoreCatena(t)
	for _, c := range []struct {
		codice string
		atteso valutazione.LetturaRegistrata
	}{
		{"7120100A2", valutazione.LetturaRegistrata{Originale: "7120100A2", Base: "7120100", Marcatore: "A", Revisione: "2", Leggibile: true}},
		{"7120100A", valutazione.LetturaRegistrata{Originale: "7120100A", Base: "7120100", Marcatore: "A", Leggibile: true}},
		{"7120100B", valutazione.LetturaRegistrata{Originale: "7120100B", Base: "7120100", Marcatore: "B", Leggibile: true}},
		{"7120100X", valutazione.LetturaRegistrata{Originale: "7120100X", Motivo: valutazione.MotivoLetturaNessunaLettura}},
		{"7120100a2", valutazione.LetturaRegistrata{Originale: "7120100a2", Motivo: valutazione.MotivoLetturaNessunaLettura}},
		{"  ", valutazione.LetturaRegistrata{Originale: "  ", Motivo: valutazione.MotivoLetturaCodiceVuoto}},
	} {
		if got := valutazione.LeggiCodiceRegistrato(m, c.codice); !reflect.DeepEqual(got, c.atteso) {
			t.Errorf("%q: %+v, atteso %+v", c.codice, got, c.atteso)
		}
	}
	if got := valutazione.LeggiCodiceRegistrato(nil, "7120100A2"); got.Leggibile || got.Base != "" || got.Motivo != valutazione.MotivoLetturaSenzaGrammatica {
		t.Errorf("senza grammatica: %+v", got)
	}
	// Due famiglie con lo stesso codice in due spazi diversi: le letture non danno la stessa base, e non si sceglie.
	catena, copia := famigliaCatena(), famigliaCatena()
	copia.ID, copia.Namespace = "acme-copia", "acme-copia"
	for _, f := range []*grammatica.FamigliaCodice{&catena, &copia} {
		for i := range f.Esempi {
			f.Esempi[i].Atteso.AltreAmmesse = true // l'altra famiglia legge lo stesso testo
		}
	}
	due := motoreConFamiglie(t, catena, copia)
	if got := valutazione.LeggiCodiceRegistrato(due, "7120100A"); got.Leggibile || got.Motivo != valutazione.MotivoLetturaBasiDiverse {
		t.Errorf("due spazi di codici: %+v", got)
	}
	// La stessa regola dei target: la base del finito manuale è quella della lettura del codice registrato.
	v := valuta(t, scenaCollegamento(t), m, nil)
	if pv := prodotto(t, v, rifProdB); pv.Base.Normalizzata != valutazione.LeggiCodiceRegistrato(m, pv.CodiceRichiesto).Base {
		t.Errorf("base del target %q, lettura del codice registrato %+v", pv.Base.Normalizzata, valutazione.LeggiCodiceRegistrato(m, pv.CodiceRichiesto))
	}
}

// scenaVecchio: la scena del collegamento con una proposta sul 2D dello sciolto che il documento confermato contraddice:
// la proposta dice il componente terzo e un altro codice, il documento dice lo sciolto. Il codice del documento è dato.
func scenaVecchio(t *testing.T, codiceDocumento string, dettagli string) fotorfq.Thread {
	t.Helper()
	th := scenaCollegamento(t)
	for i := range th.Documenti {
		if th.Documenti[i].ID == dDisegno {
			th.Documenti[i].Codice = testo(codiceDocumento)
		}
	}
	terzo := cTerzo
	il := time.Date(2026, 10, 2, 9, 30, 0, 123000000, time.UTC)
	op := operatore
	th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: uid(0x742), AllegatoID: aDisegno, Tipo: "disegno_2d", Codice: testo("7120299A1"),
		Rev: testo("1"), ComponenteID: &terzo, Fonte: "cartiglio", Stato: "confermata", DecisoDa: &op, DecisoIl: &il, Dettagli: json.RawMessage(dettagli)})
	return th
}

// TestL130IlVecchioDiOgniFile (A1c-L1-30; registro §10.2; T-B0-14, F0-02; R31 c): il vecchio di un file deciso viene dal
// documento (componente, codice, documento, sostituzione), la proposta dà stato, fonte, «assegna», la decisione e i
// dettagli; il codice si legge con la grammatica, e se non si legge lo dice la diagnostica, mai la stringa.
func TestL130IlVecchioDiOgniFile(t *testing.T) {
	r := insiemeACME(t)
	nodo := "nodo:" + sha("a") + ":#2"
	dettagli := `{"valutazione": {"v": 2, "tabella": "t", "codice": {"valore": "7120299A1", "score": 80, "regola": "cartiglio", "stato": "unica", "evidenze": []}},` +
		` "destinazione": {"v": 1, "candidati": [{"chiave": "componente:` + cSciolto.String() + `"}, {"chiave": "` + nodo + `"}, {"chiave": "componente:` + cSciolto.String() + `"}]}}`

	t.Run("il file deciso", func(t *testing.T) {
		e := calcola(t, fotografiaDi(scenaVecchio(t, "7120200A1", dettagli)), r, valutazione.Ingressi{})
		et := esitoThread(t, e, threadACME)
		v := confrontabileDi(t, et, aDisegno).Vecchio
		il := time.Date(2026, 10, 2, 9, 30, 0, 123000000, time.UTC)
		atteso := valutazione.VecchioPiatto{Stato: "confermata", Fonte: "cartiglio", Codice: "7120200A1", Rev: "1", Base: "7120200", Marcatore: "A",
			Revisione: "1", Leggibile: true, CodiceLetto: "7120299A1", CodiceLettoBase: "7120299", CodiceLettoMarcatore: "A", Componente: ptr(cSciolto), Documento: ptr(dDisegno),
			ComponenteProposta: ptr(cTerzo), DecisoIl: &il, Destinazione: []string{"componente:" + cSciolto.String(), nodo}}
		if !reflect.DeepEqual(v, atteso) {
			t.Errorf("vecchio\n%+v\natteso\n%+v", v, atteso)
		}
		if n := confrontabileDi(t, et, aDisegno).Nuovo; n.Revisione != "1" || n.Revisioni != valutazione.RevisioniUguali || n.MotivoRevisioni != "" {
			t.Errorf("revisioni del 2D: %+v", n)
		}
		if contieneCodice(et.Diagnostiche, valutazione.CodiceCodiceRegistratoNonLeggibile) {
			t.Error("un codice che si legge non dà la diagnostica")
		}
		// la scansione: solo la proposta aperta con «assegna», senza codice e senza documento
		s := confrontabileDi(t, et, aScansione).Vecchio
		if s.Stato != "aperta" || s.Fonte != "operatore" || s.ComponenteProposta == nil || *s.ComponenteProposta != cSciolto || s.Componente != nil ||
			s.Documento != nil || s.Codice != "" || s.MotivoLettura != valutazione.MotivoLetturaCodiceVuoto || s.Destinazione != nil || s.CodiceLetto != "" {
			t.Errorf("vecchio della scansione: %+v", s)
		}
		// un file senza proposta né documento: il vecchio è vuoto
		if n := confrontabileDi(t, et, aInAttesa).Vecchio; n.Stato != "" || n.Codice != "" || n.Leggibile {
			t.Errorf("vecchio di un file senza proposta: %+v", n)
		}
	})
	t.Run("le revisioni confrontate con la grammatica", func(t *testing.T) {
		for _, c := range []struct {
			codice, revisioni, motivo string
		}{
			{"7120200A2", valutazione.RevisioniDiverse, ""},
			// R-65 della revisione di V1: il codice letto senza revisione, la revisione solo nella colonna rev del documento
			{"7120200A", valutazione.RevisioniNonConfrontabili, valutazione.MotivoRevisioniSoloInColonna},
			// il codice che non si legge: la revisione vecchia non è letta, qualunque cosa dica la colonna
			{"7120200X1", valutazione.RevisioniNonConfrontabili, valutazione.MotivoRevisioniVecchiaNonLetta},
		} {
			e := calcola(t, fotografiaDi(scenaVecchio(t, c.codice, dettagli)), r, valutazione.Ingressi{})
			n := confrontabileDi(t, esitoThread(t, e, threadACME), aDisegno).Nuovo
			if n.Revisioni != c.revisioni || n.MotivoRevisioni != c.motivo {
				t.Errorf("documento %q: revisioni %q/%q, attese %q/%q", c.codice, n.Revisioni, n.MotivoRevisioni, c.revisioni, c.motivo)
			}
		}
		// il nuovo con due revisioni diverse (il nome e il cartiglio), e senza revisione
		th := scenaVecchio(t, "7120200A1", dettagli)
		disc, senza := uid(0x6c7), uid(0x6c8)
		fd := fattiPDF(t, sha("3"), "7120200A1")
		conAllegato(&th, allegato(disc, 14, "7120200A_2.pdf", "pdf", sha("3")), &fd)
		fn := fattiPDF(t, sha("4"), "7120300B")
		conAllegato(&th, allegato(senza, 15, "senza-rev-acme.pdf", "pdf", sha("4")), &fn)
		th.Proposte = append(th.Proposte,
			fotorfq.PropostaAttuale{ID: uid(0x6c9), AllegatoID: disc, Tipo: "disegno_2d", Codice: testo("7120200A1"), Fonte: "cartiglio", Stato: "aperta"},
			fotorfq.PropostaAttuale{ID: uid(0x6ca), AllegatoID: senza, Tipo: "disegno_2d", Codice: testo("7120300B2"), Fonte: "cartiglio", Stato: "aperta"})
		et := esitoThread(t, calcola(t, fotografiaDi(th), r, valutazione.Ingressi{}), threadACME)
		if n := confrontabileDi(t, et, disc).Nuovo; n.Revisione != "" || n.Revisioni != valutazione.RevisioniNonConfrontabili || n.MotivoRevisioni != valutazione.MotivoRevisioniNuoveDiscordi {
			t.Errorf("due revisioni nuove diverse: %+v", n)
		}
		if n := confrontabileDi(t, et, senza).Nuovo; n.Revisione != "" || n.Revisioni != valutazione.RevisioniNonConfrontabili || n.MotivoRevisioni != valutazione.MotivoRevisioniNuovaNonLetta {
			t.Errorf("nessuna revisione nuova: %+v", n)
		}
	})
	t.Run("il codice che non si legge", func(t *testing.T) {
		for _, codice := range []string{"7120200X1", "7120200a1"} {
			e := calcola(t, fotografiaDi(scenaVecchio(t, codice, dettagli)), r, valutazione.Ingressi{})
			et := esitoThread(t, e, threadACME)
			v := confrontabileDi(t, et, aDisegno).Vecchio
			if v.Leggibile || v.Base != "" || v.Marcatore != "" || v.MotivoLettura != valutazione.MotivoLetturaNessunaLettura || v.Codice != codice {
				t.Errorf("%q: vecchio %+v (nessun confronto per stringa)", codice, v)
			}
			d := conCodice(et.Diagnostiche, valutazione.CodiceCodiceRegistratoNonLeggibile)
			if len(d) != 1 || !reflect.DeepEqual(d[0].Rif, []string{aDisegno.String()}) {
				t.Errorf("%q: diagnostiche %+v", codice, d)
			}
		}
	})
	t.Run("la lettura del vecchio motore", func(t *testing.T) {
		operatore := `{"valutazione": {"v": 2, "codice": {"valore": "7120200A1", "regola": "operatore"}}}`
		for _, c := range []struct{ dettagli, atteso, base, marcatore string }{
			{operatore, "", "", ""},
			{`{"valutazione": {"v": 2, "codice": "7120200A1"}}`, "", "", ""},
			{`{"altro": 1}`, "", "", ""},
			{`{"valutazione": {"v": 2, "codice": {"valore": "7120200A1", "regola": "nome_file"}}}`, "7120200A1", "7120200", "A"},
			// R-64 della revisione di V1: si conoscono solo le versioni 1 e 2; per la v1 vale il valore registrato allora
			{`{"valutazione": {"v": 1, "codice": {"valore": "7120201A1", "regola": "nome_codice_famiglia"}}}`, "7120201A1", "7120201", "A"},
			{`{"valutazione": {"v": 99, "codice": {"valore": "7120200A1", "regola": "cartiglio"}}}`, "", "", ""},
			{`{"valutazione": {"codice": {"valore": "7120200A1", "regola": "nome_file"}}}`, "", "", ""},
			// F0-19: la base della lettura del vecchio motore, con la grammatica; vuota se non si legge, e il marcatore
			// con lei
			{`{"valutazione": {"v": 2, "codice": {"valore": "7120200X1", "regola": "cartiglio"}}}`, "7120200X1", "", ""},
			// R114, precisata il 07/10: il marcatore della lettura del vecchio motore, non quello del codice registrato del
			// documento (7120200A1)
			{`{"valutazione": {"v": 2, "codice": {"valore": "7120200B1", "regola": "cartiglio"}}}`, "7120200B1", "7120200", "B"},
		} {
			e := calcola(t, fotografiaDi(scenaVecchio(t, "7120200A1", c.dettagli)), r, valutazione.Ingressi{})
			v := confrontabileDi(t, esitoThread(t, e, threadACME), aDisegno).Vecchio
			if v.CodiceLetto != c.atteso || v.CodiceLettoBase != c.base || v.CodiceLettoMarcatore != c.marcatore || v.Destinazione != nil {
				t.Errorf("%s: codice letto %q (base %q, marcatore %q), destinazione %v", c.dettagli, v.CodiceLetto, v.CodiceLettoBase,
					v.CodiceLettoMarcatore, v.Destinazione)
			}
		}
	})
	t.Run("il documento sostituito e quello corrente", func(t *testing.T) {
		// Il 2D dello sciolto è stato sostituito da un documento nuovo dello stesso allegato: per il 2D vale il documento
		// corrente, anche se il suo ID viene dopo. Una copia che porta solo il documento sostituito ha quello, con la
		// sostituzione.
		th := scenaVecchio(t, "7120200A1", dettagli)
		nuovo, soloVecchio := uid(0x7cb), uid(0x7cc)
		fv := fattiPDF(t, sha("d"), "7120200A1")
		conAllegato(&th, allegato(soloVecchio, 17, "copia-acme.pdf", "pdf", sha("d")), &fv)
		for i := range th.Documenti {
			if th.Documenti[i].ID == dDisegno {
				th.Documenti[i].SostituitoDa = &nuovo
				th.Documenti[i].Allegati = append(th.Documenti[i].Allegati, soloVecchio)
			}
		}
		th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: nuovo, ThreadID: threadACME, ComponenteID: ptr(cSciolto), Tipo: "disegno_2d",
			Codice: testo("7120200A2"), NomeFile: "disegno-acme.pdf", Estensione: "pdf", Sha256: sha("b"), StatoNas: "scritto", ConfermatoDa: operatore,
			ConfermatoIl: dataACME, Allegati: []uuid.UUID{aDisegno}})
		et := esitoThread(t, calcola(t, fotografiaDi(th), r, valutazione.Ingressi{}), threadACME)
		if v := confrontabileDi(t, et, aDisegno).Vecchio; v.Documento == nil || *v.Documento != nuovo || v.SostituitoDa != nil || v.Codice != "7120200A2" {
			t.Errorf("il 2D: vale il documento corrente: %+v", v)
		}
		if v := confrontabileDi(t, et, soloVecchio).Vecchio; v.Documento == nil || *v.Documento != dDisegno || v.SostituitoDa == nil || *v.SostituitoDa != nuovo {
			t.Errorf("la copia che porta solo il documento sostituito: %+v", v)
		}
	})
}

// TestIlVecchioDeiProdotti (par.3.3.8, VecchiProdotti; F0-18): gli identificativi confermati e i candidati di codice della
// fotografia, in ordine, senza doppioni; un identificativo solo proposto non c'è.
func TestIlVecchioDeiProdotti(t *testing.T) {
	th := scenaCollegamento(t)
	th.Identificativi = []fotorfq.Identificativo{confermato("P7120100"), proposto("P7120200"), confermato("P7120100")}
	th.CandidatiCodice = []fotorfq.CandidatoCodice{{MessaggioID: idM1, Codice: "7120300A", Ruolo: "prodotto", Origine: "corpo"},
		{MessaggioID: idM1, Codice: "7120100A", Ruolo: "prodotto", Origine: "oggetto"}}
	et := esitoThread(t, calcola(t, fotografiaDi(th), insiemeACME(t), valutazione.Ingressi{}), threadACME)
	atteso := []string{"candidato:7120100A", "candidato:7120300A", "identificativo:P7120100"}
	if !reflect.DeepEqual(et.VecchiProdotti, atteso) {
		t.Errorf("vecchi prodotti %v, attesi %v", et.VecchiProdotti, atteso)
	}
}
