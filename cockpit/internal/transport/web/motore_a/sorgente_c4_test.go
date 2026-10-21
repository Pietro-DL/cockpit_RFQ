package motore_a

// L1 — direttiva 35, blocco C-4: le righe di S9 dalle bozze di tutti i prodotti e dalle AR del servizio unico
// (sorgente.go; piano delle dipendenze della direttiva 35 §7.2 C-4; EM §10.9 «S9»), sulla scena ACME inventata con i
// due prodotti (indice_test.go: 7120001 con l'albero 10 → 11 → 12 → 14, 7120002 senza figli; qui in più l'arco
// 7120002 → 7120013, così lo sciolto 7120013 è un componente condiviso fra i due prodotti). La lettura delle bozze
// (DatiV1.LetturaBozze) è scritta a mano, come la darebbe distintabozza.LeggiBozzeDellaRfq.
//   - le quattro fonti distinte: proposta del motore, PB salvata (chiave_bozza e casella veri), AR registrata, documento
//     confermato senza AR; gli svincoli pendenti (anche già chiusi) e i file da ricollocare (con e senza il nodo);
//   - i conflitti: il file conteso fra due bozze (nelle due sezioni), il file già associato altrove, lo svincolo nella
//     stessa bozza che fa della PB una sostituzione;
//   - la condivisione: lo stesso componente in due prodotti (stesso_componente) distinto dal file pertinente a due
//     prodotti con destinazioni diverse (file_pertinente);
//   - i conti per stato, le azioni per ciò che la riga è, la firma che cambia con una PB; la correzione del codice di un
//     nodo non sposta la riga (R-38); un file con una PB o una AR non è più da associare;
//   - la copia di documento_proposta non fa nascere righe registrate (mai la copia come fonte delle AR).
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, UUID di prova).

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/valutazione"
)

// I file della scena C-4.
var (
	c4Condiviso    = maUUID(701) // AR su 7120013 (condiviso fra i due prodotti)
	c4Salvato      = maUUID(702) // PB su un nodo nuovo della bozza del primo prodotto
	c4Conteso      = maUUID(703) // PB in tutte e due le bozze, destinazioni diverse
	c4Altrove      = maUUID(704) // AR su 7120010 (primo prodotto), PB nella bozza del secondo
	c4Confermata   = maUUID(705) // AR su 7120011 e PB della bozza del primo sullo stesso componente
	c4Sostituzione = maUUID(706) // AR su 7120011, svincolo e PB su un nodo nuovo nella bozza del primo
	c4GiaChiuso    = maUUID(707) // svincolo della bozza del primo senza AR attiva (chiusa dalla corsia file)
	c4Ricollocare  = maUUID(708) // vassoio della bozza del primo, il nodo d'uscita c'è
	c4NodoTolto    = maUUID(709) // vassoio della bozza del primo, il nodo d'uscita non c'è più
	c4Proposto     = maUUID(710) // proposta del motore verso 7120013, pertinente ai due prodotti
	c4Documento    = maUUID(711) // documento confermato su 7120010, senza AR
	c4Copia        = maUUID(712) // solo la copia di documento_proposta su 7120013, senza AR
	c4Step         = maUUID(31)  // lo STEP della scena, per le caselle
)

// risultatoC4: la scena ACME con l'arco condiviso, le associazioni del motore, le due bozze formato 3 e le AR.
func risultatoC4() *Risultato {
	r := maRisultatoACME()
	r.Thread.Relazioni = append(r.Thread.Relazioni, fotorfq.Relazione{PadreID: maUUID(20), FiglioID: maUUID(14), Qta: 1, Origine: "manuale"})
	comp := func(nn int) string { return "componente/" + maComp(nn) }
	r.EsitoThread.Associazioni = append(r.EsitoThread.Associazioni,
		valutazione.AssociazioneFile{AllegatoID: c4Proposto, Proposta: []string{comp(14)}, Pertinente: []string{maP1, maP2}},
		valutazione.AssociazioneFile{AllegatoID: c4Documento, Confermata: maUUIDp(11), DocumentoID: maUUIDp(811), Terminale: true,
			Pertinente: []string{maP1}},
	)
	r.Thread.Proposte = append(r.Thread.Proposte, fotorfq.PropostaAttuale{ID: maUUID(812), AllegatoID: c4Copia, Tipo: "disegno_2d",
		Fonte: "nome_file", Stato: "aperta", ComponenteID: maUUIDp(14)})
	casella := &distintabozza.Casella3{AllegatoID: c4Step, Gruppo: "bom", Rif: "nodo:prova:#5"}
	nodo := func(chiave, codice string, componente *uuid.UUID, cas *distintabozza.Casella3) distintabozza.NodoDellaBozza {
		return distintabozza.NodoDellaBozza{ChiaveBozza: chiave, ComponenteID: componente, Codice: codice, Rev: "A", Casella: cas}
	}
	pb := func(a uuid.UUID, n distintabozza.NodoDellaBozza) distintabozza.PreassociazioneDellaBozza {
		return distintabozza.PreassociazioneDellaBozza{AllegatoID: a, ChiaveBozza: n.ChiaveBozza, Nodo: n}
	}
	ritorno := nodo("k-e", "7120060", nil, casella)
	// direttiva 39 (D-AR11 B [U]): le proposte del motore ci sono solo per i prodotti con la sorgente confermata da S8; la
	// scena di C-4 le vuole nei due prodotti (le regole nuove sono in sorgente_d39_test.go)
	r.V1.SorgentiS8 = map[uuid.UUID]SorgenteS8{
		maUUID(10): {AllegatoID: c4Step, Sha256: maSha(1), Stato: confermabom.StatoSorgente{Valida: true, Documento: maUUID(41)}},
		maUUID(20): {AllegatoID: maUUID(39), Sha256: maSha(39), Stato: confermabom.StatoSorgente{Valida: true, Documento: maUUID(49)}},
	}
	r.V1.LetturaBozze = distintabozza.BozzeDellaRfq{
		Bozze: []distintabozza.BozzaDellaRfq{
			{ProdottoID: maUUID(10), BozzaID: maUUID(901), Formato: 3, Versione: 4,
				Preassociazioni: []distintabozza.PreassociazioneDellaBozza{
					pb(c4Salvato, nodo("k-a", "7120050", nil, casella)),
					pb(c4Conteso, nodo("k-b", "7120011", maUUIDp(12), nil)),
					pb(c4Confermata, nodo("k-c", "7120011", maUUIDp(12), nil)),
					pb(c4Sostituzione, nodo("k-d", "7120055", nil, nil)),
				},
				Svincoli: []distintabozza.SvincoloPendente{
					{DecisioneID: "sv-1", AllegatoID: c4Sostituzione, ComponenteID: maUUID(12), AssociazioneAttiva: true},
					{DecisioneID: "sv-2", AllegatoID: c4GiaChiuso, ComponenteID: maUUID(11), AssociazioneAttiva: false},
				},
				Vassoio: []distintabozza.VoceVassoioDellaBozza{
					{AllegatoID: c4Ricollocare, Motivo: "nodo_escluso", ChiaveBozza: "k-e", Nodo: &ritorno},
					{AllegatoID: c4NodoTolto, Motivo: "nodo_tolto", ChiaveBozza: "k-x"},
				}},
			{ProdottoID: maUUID(20), BozzaID: maUUID(902), Formato: 3, Versione: 2,
				Preassociazioni: []distintabozza.PreassociazioneDellaBozza{
					pb(c4Conteso, nodo("k-p", "7120070", nil, nil)),
					pb(c4Altrove, nodo("k-q", "7120071", nil, nil)),
				},
				Svincoli: []distintabozza.SvincoloPendente{}, Vassoio: []distintabozza.VoceVassoioDellaBozza{}},
		},
		Associazioni: []associazioni.Associazione{
			{Thread: maUUID(1), Allegato: c4Condiviso, Componente: maUUID(14), Stato: associazioni.StatoAttiva},
			{Thread: maUUID(1), Allegato: c4Altrove, Componente: maUUID(11), Stato: associazioni.StatoAttiva},
			{Thread: maUUID(1), Allegato: c4Confermata, Componente: maUUID(12), Stato: associazioni.StatoAttiva},
			{Thread: maUUID(1), Allegato: c4Sostituzione, Componente: maUUID(12), Stato: associazioni.StatoAttiva},
		},
	}
	return r
}

// righeDi: le righe di un allegato nella sezione di un prodotto.
func righeDi(s *Sorgente, prodotto uuid.UUID, allegato uuid.UUID) []RigaSorgente {
	var out []RigaSorgente
	for _, p := range s.Prodotti {
		if p.ProdottoID == nil || *p.ProdottoID != prodotto {
			continue
		}
		for _, r := range p.Righe {
			if r.AllegatoID == allegato {
				out = append(out, r)
			}
		}
	}
	return out
}

func sezioneDi(t *testing.T, s *Sorgente, prodotto uuid.UUID) ProdottoSorgente {
	t.Helper()
	for _, p := range s.Prodotti {
		if p.ProdottoID != nil && *p.ProdottoID == prodotto {
			return p
		}
	}
	t.Fatalf("nessuna sezione per %s", prodotto)
	return ProdottoSorgente{}
}

// unaRiga: l'unica riga di un allegato in una sezione, con lo stato, il motivo e l'origine attesi.
func unaRiga(t *testing.T, s *Sorgente, prodotto, allegato uuid.UUID, stato, motivo, origine string) RigaSorgente {
	t.Helper()
	rr := righeDi(s, prodotto, allegato)
	if len(rr) != 1 || rr[0].Stato != stato || rr[0].Motivo != motivo || rr[0].Origine != origine {
		t.Fatalf("l'allegato %s nel prodotto %s: %+v (atteso %s/%s/%s)", allegato, prodotto, rr, stato, motivo, origine)
	}
	return rr[0]
}

func TestV1C4LeRigheDiS9DalleBozzeEDalleAR(t *testing.T) {
	r := risultatoC4()
	s := ComponiSorgente(r, maUUID(1))
	p1, p2 := maUUID(10), maUUID(20)

	// la PB salvata: chiave_bozza e casella veri, codice e rev del nodo, mai proposta_id come riferimento
	x := unaRiga(t, s, p1, c4Salvato, StatoRigaSalvato, "", OrigineRigaBozza)
	if x.Destinazione.ChiaveBozza != "k-a" || x.Destinazione.Casella == nil || x.Destinazione.Casella.AllegatoID != c4Step ||
		x.Destinazione.Casella.Rif != "nodo:prova:#5" || x.Destinazione.ComponenteID != nil || x.CodiceMostrato != "7120050" || x.RevMostrata != "A" ||
		!x.Azioni.TogliDallaBozza.Disponibile || !x.Azioni.Sposta.Disponibile || x.Azioni.SalvaNellaBozza.Disponibile {
		t.Errorf("la PB salvata: %+v", x)
	}
	// il file conteso fra le due bozze: in_conflitto nelle due sezioni, file_pertinente
	for _, p := range []uuid.UUID{p1, p2} {
		x := unaRiga(t, s, p, c4Conteso, StatoRigaInConflitto, MotivoRigaFileConteso, OrigineRigaBozza)
		if len(x.Condivisione) != 1 || x.Condivisione[0].Motivo != CondivisioneFilePertinente || !x.Azioni.TogliDallaBozza.Disponibile ||
			x.Azioni.SalvaNellaBozza.Disponibile {
			t.Errorf("il file conteso nel prodotto %s: %+v", p, x)
		}
	}
	// il file già associato altrove: la PB del secondo prodotto è in conflitto, il primo prodotto lo ha registrato
	x = unaRiga(t, s, p2, c4Altrove, StatoRigaInConflitto, MotivoRigaGiaAssociataAltrove, OrigineRigaBozza)
	if x.Destinazione.ChiaveBozza != "k-q" || x.Azioni.SalvaNellaBozza.Motivo != MotivoAzioneSvincoloAltrove {
		t.Errorf("già associata altrove: %+v", x)
	}
	x = unaRiga(t, s, p1, c4Altrove, StatoRigaRegistrato, "", OrigineRigaWorking)
	if x.Destinazione.ComponenteID == nil || *x.Destinazione.ComponenteID != maUUID(11) || x.CodiceMostrato != "7120010" {
		t.Errorf("la AR nel prodotto che contiene il componente: %+v", x)
	}
	// la PB che coincide con la AR: registrata, con la chiave della bozza, e si può togliere dalla bozza
	x = unaRiga(t, s, p1, c4Confermata, StatoRigaRegistrato, "", OrigineRigaBozza)
	if x.Destinazione.ChiaveBozza != "k-c" || x.Destinazione.ComponenteID == nil || *x.Destinazione.ComponenteID != maUUID(12) ||
		!x.Azioni.TogliDallaBozza.Disponibile || !x.Azioni.ScollegaAllaConferma.Disponibile {
		t.Errorf("la PB che conferma la AR: %+v", x)
	}
	// lo svincolo e la PB nella stessa bozza: la sostituzione (da_scollegare sul vecchio componente, salvato sul nodo nuovo)
	rr := righeDi(s, p1, c4Sostituzione)
	if len(rr) != 2 || rr[0].Stato != StatoRigaDaScollegare || rr[0].Motivo != "" || rr[0].Destinazione.ComponenteID == nil ||
		*rr[0].Destinazione.ComponenteID != maUUID(12) || rr[1].Stato != StatoRigaSalvato || rr[1].Destinazione.ChiaveBozza != "k-d" ||
		!rr[0].Azioni.TogliDallaBozza.Disponibile || rr[0].Azioni.Sposta.Disponibile {
		t.Errorf("la sostituzione: %+v", rr)
	}
	// lo svincolo di una AR già chiusa (D-ASS-03): ancora da_scollegare, con il motivo
	unaRiga(t, s, p1, c4GiaChiuso, StatoRigaDaScollegare, MotivoRigaSvincoloGiaChiuso, OrigineRigaBozza)
	// i file da ricollocare: nessuna destinazione, l'uscita con il nodo (o la sola chiave se il nodo non c'è più)
	x = unaRiga(t, s, p1, c4Ricollocare, StatoRigaDaRicollocare, "nodo_escluso", OrigineRigaBozza)
	if x.Destinazione != (DestinazioneRiga{}) || x.Uscita == nil || x.Uscita.ChiaveBozza != "k-e" || x.Uscita.Casella == nil ||
		x.CodiceMostrato != "7120060" || !x.Azioni.Sposta.Disponibile || x.Azioni.SalvaNellaBozza.Disponibile {
		t.Errorf("il file da ricollocare: %+v", x)
	}
	x = unaRiga(t, s, p1, c4NodoTolto, StatoRigaDaRicollocare, "nodo_tolto", OrigineRigaBozza)
	if x.Uscita == nil || x.Uscita.ChiaveBozza != "k-x" || x.Uscita.Casella != nil || x.CodiceMostrato != "" {
		t.Errorf("il file da ricollocare dal nodo tolto: %+v", x)
	}
	// la AR sul componente condiviso: registrata nelle due sezioni, stesso_componente
	for _, p := range []uuid.UUID{p1, p2} {
		x := unaRiga(t, s, p, c4Condiviso, StatoRigaRegistrato, "", OrigineRigaWorking)
		if len(x.Condivisione) != 1 || x.Condivisione[0].Motivo != CondivisioneStessoComponente || len(x.CondivisoCon) != 1 {
			t.Errorf("il componente condiviso nel prodotto %s: %+v", p, x)
		}
	}
	// la proposta del motore verso il componente condiviso: proposta nelle due sezioni, stesso_componente
	for _, p := range []uuid.UUID{p1, p2} {
		x := unaRiga(t, s, p, c4Proposto, StatoRigaProposto, "", OrigineRigaMotore)
		if len(x.Condivisione) != 1 || x.Condivisione[0].Motivo != CondivisioneStessoComponente {
			t.Errorf("la proposta verso il componente condiviso nel prodotto %s: %+v", p, x)
		}
	}
	// il documento confermato senza AR: registrato con il motivo, contato fra i confermati; non si sposta da qui
	x = unaRiga(t, s, p1, c4Documento, StatoRigaRegistrato, MotivoRigaDocumentoConfermato, OrigineRigaWorking)
	if x.Azioni.Sposta.Disponibile || x.Azioni.ScollegaAllaConferma.Motivo != MotivoAzioneDocumento {
		t.Errorf("il documento confermato: %+v", x)
	}
	// un file con una PB o una AR non è da associare, anche se il motore lo dà da smistare
	r6 := risultatoC4()
	r6.EsitoThread.DaSmistare = append(r6.EsitoThread.DaSmistare, valutazione.FileDaSmistare{AllegatoID: c4Salvato, Motivo: valutazione.MotivoSmistamentoNessunCandidato},
		valutazione.FileDaSmistare{AllegatoID: c4Condiviso, Motivo: valutazione.MotivoSmistamentoNessunCandidato},
		valutazione.FileDaSmistare{AllegatoID: c4Copia, Motivo: valutazione.MotivoSmistamentoNessunCandidato})
	s6 := ComponiSorgente(r6, maUUID(1))
	if len(s6.DaAssociare) != len(s.DaAssociare)+1 || s6.DaAssociare[len(s6.DaAssociare)-1].AllegatoID != c4Copia {
		t.Errorf("da associare con una PB e una AR: %+v", s6.DaAssociare)
	}
	// la copia senza AR non fa nascere righe
	for _, p := range []uuid.UUID{p1, p2} {
		if rr := righeDi(s, p, c4Copia); len(rr) != 0 {
			t.Errorf("la copia di documento_proposta ha fatto nascere righe nel prodotto %s: %+v", p, rr)
		}
	}

	// i conti per stato
	if c := sezioneDi(t, s, p1).Conti; c.Salvati != 2 || c.Registrati != 3 || c.Confermati != 1 || c.DaScollegare != 2 || c.DaRicollocare != 2 ||
		c.InConflitto != 1 || c.Proposti != 1 {
		t.Errorf("i conti del primo prodotto: %+v", c)
	}
	if c := sezioneDi(t, s, p2).Conti; c.Salvati != 0 || c.Registrati != 1 || c.InConflitto != 2 || c.Proposti != 1 || c.DaScollegare != 0 {
		t.Errorf("i conti del secondo prodotto: %+v", c)
	}
	for _, p := range s.Prodotti {
		n := map[string]int{}
		for _, r := range p.Righe {
			k := r.Stato
			if r.Motivo == MotivoRigaDocumentoConfermato {
				k = "confermato"
			}
			n[k]++
		}
		c := p.Conti
		if n[StatoRigaProposto] != c.Proposti || n[StatoRigaSalvato] != c.Salvati || n[StatoRigaRegistrato] != c.Registrati || n["confermato"] != c.Confermati ||
			n[StatoRigaDaScollegare] != c.DaScollegare || n[StatoRigaDaRicollocare] != c.DaRicollocare || n[StatoRigaInConflitto] != c.InConflitto {
			t.Errorf("i conti di %s non sono le righe: %+v / %v", p.Prodotto, c, n)
		}
		for _, r := range p.Righe {
			if len(r.CondivisoCon) != len(r.Condivisione) {
				t.Errorf("condiviso_con e condivisione non vanno insieme: %+v", r)
			}
			for i := range r.CondivisoCon {
				if r.CondivisoCon[i] != r.Condivisione[i].ProdottoID {
					t.Errorf("condiviso_con e condivisione in ordine diverso: %+v", r)
				}
			}
		}
	}

	// la firma: la stessa a ogni composizione, diversa con una PB in più
	if s2 := ComponiSorgente(risultatoC4(), maUUID(1)); s2.Firma != s.Firma {
		t.Error("la firma cambia senza cambi")
	}
	r3 := risultatoC4()
	b := &r3.V1.LetturaBozze.Bozze[1]
	b.Preassociazioni = append(b.Preassociazioni, distintabozza.PreassociazioneDellaBozza{AllegatoID: c4Copia, ChiaveBozza: "k-p",
		Nodo: distintabozza.NodoDellaBozza{ChiaveBozza: "k-p", Codice: "7120070"}})
	s3 := ComponiSorgente(r3, maUUID(1))
	if s3.Firma == s.Firma || sezioneDi(t, s3, p2).Conti.Salvati != 1 {
		t.Errorf("una PB in più: firma uguale %v, conti %+v", s3.Firma == s.Firma, sezioneDi(t, s3, p2).Conti)
	}

	// R-38: la correzione del codice del nodo non sposta la riga (la destinazione è la chiave_bozza, mai il codice)
	r7 := risultatoC4()
	r7.V1.LetturaBozze.Bozze[0].Preassociazioni[0].Nodo.Codice = "7120051"
	s7 := ComponiSorgente(r7, maUUID(1))
	x7 := unaRiga(t, s7, p1, c4Salvato, StatoRigaSalvato, "", OrigineRigaBozza)
	x0 := unaRiga(t, s, p1, c4Salvato, StatoRigaSalvato, "", OrigineRigaBozza)
	if x7.CodiceMostrato != "7120051" || x7.Destinazione.ChiaveBozza != x0.Destinazione.ChiaveBozza || *x7.Destinazione.Casella != *x0.Destinazione.Casella ||
		s7.Firma == s.Firma {
		t.Errorf("la correzione del codice del nodo: %+v / %+v", x7, x0)
	}
	// senza bozze né AR: le righe sono quelle del motore e la firma è quella della scena senza la lettura
	r4 := risultatoC4()
	r4.V1.LetturaBozze = distintabozza.BozzeDellaRfq{}
	s4 := ComponiSorgente(r4, maUUID(1))
	for _, p := range s4.Prodotti {
		for _, r := range p.Righe {
			if r.Origine == OrigineRigaBozza || (r.Origine == OrigineRigaWorking && r.Motivo != MotivoRigaDocumentoConfermato) {
				t.Errorf("senza la lettura delle bozze una riga della bozza o una AR: %+v", r)
			}
		}
	}
	// una bozza formato 2 (da convertire) non dà righe della bozza
	r5 := risultatoC4()
	r5.V1.LetturaBozze.Bozze[1] = distintabozza.BozzaDellaRfq{ProdottoID: maUUID(20), BozzaID: maUUID(903), Formato: 2, DaConvertire: true}
	s5 := ComponiSorgente(r5, maUUID(1))
	if rr := righeDi(s5, p2, c4Altrove); len(rr) != 0 {
		t.Errorf("una bozza formato 2 dà righe: %+v", rr)
	}
	unaRiga(t, s5, p1, c4Conteso, StatoRigaSalvato, "", OrigineRigaBozza)
}
