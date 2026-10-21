package motore_a

// L1 — direttiva 39, corsia motore: la fonte del prodotto in /dati (e quindi nella sezione di S9) con S8.
//   - «Conferma la fonte» è S8 e non dipende più dal legame della proposta dello STEP con il prodotto
//     (documento_proposta.componente_id): conferma_ammessa con il gesto conferma_sorgente anche senza proposta, con la
//     proposta su un altro componente o senza componente;
//   - la sorgente valida di S8 (confermabom.SorgenteValida, letta in leggiV1): sorgente_confermata, il motivo «già
//     confermata», conferma non più ammessa; una sorgente revocata non vale (sorgente_s8.valida falso, motivo revocata) e
//     S8 è di nuovo ammessa;
//   - lo STEP storico univoco di D-AR04 (iii): legame_storico distinto, solo per un prodotto senza conferme di S8 e con un
//     solo documento cad_3d STEP corrente; non è una sorgente confermata;
//   - lo STEP che è già il documento di un altro componente: conferma non ammessa, con il motivo;
//   - struttura_step c'è sempre con uno STEP;
//   - datiDellaFonte: solo la revoca dell'autorizzazione dello stesso documento, senza la chiave di S8, toglie
//     l'autorizzazione (R39-10: la revoca della sorgente della bozza di S8 non tocca la working).
// Scena ACME inventata (indice_test.go); il prodotto P1 ha il documento cad_3d 41 nato dall'allegato 31.

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
)

func maFonteP1(r *Risultato) *FonteDelProdotto {
	return ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Prodotto: maP1, Scrive: true}).Fonte
}

func TestV1D39LaFonteEConfermaLaSorgenteConS8(t *testing.T) {
	pid, a31, doc41 := maUUID(10), maUUID(31), maUUID(41)

	// senza S8: la conferma è ammessa ed è S8; il documento legacy resta «confermata» e lo STEP storico univoco si vede
	r := maRisultatoACME()
	f := maFonteP1(r)
	if f == nil || f.AllegatoID == nil || *f.AllegatoID != a31 {
		t.Fatalf("la fonte di P1 è lo STEP 31: %+v", f)
	}
	if !f.ConfermaAmmessa || f.Gesto != GestoConfermaSorgente || f.SorgenteConfermata || f.SorgenteS8 != nil || !f.Confermata || f.Motivo != "" {
		t.Errorf("senza S8: %+v", f)
	}
	if f.LegameStorico == nil || f.LegameStorico.DocumentoID != doc41 || f.LegameStorico.AllegatoID == nil || *f.LegameStorico.AllegatoID != a31 {
		t.Errorf("lo STEP storico univoco: %+v", f.LegameStorico)
	}
	if f.StrutturaSTEP == nil || f.StrutturaSTEP.StatoAnalisi == "" || f.StrutturaSTEP.Partenza == "" {
		t.Errorf("struttura_step: %+v", f.StrutturaSTEP)
	}

	// il legame della proposta non conta: nessuna proposta, la proposta senza componente o su un altro componente
	for nome, cambia := range map[string]func(*Risultato){
		"senza proposta": func(r *Risultato) { r.Thread.Proposte = nil },
		"proposta senza componente": func(r *Risultato) {
			r.Thread.Proposte = append(r.Thread.Proposte, fotorfq.PropostaAttuale{ID: maUUID(59), AllegatoID: a31, Tipo: tipoDocumentoCad3D, Stato: "aperta"})
		},
		"proposta su un altro componente": func(r *Risultato) {
			r.Thread.Proposte = append(r.Thread.Proposte, fotorfq.PropostaAttuale{ID: maUUID(59), AllegatoID: a31, Tipo: tipoDocumentoCad3D, Stato: "aperta",
				ComponenteID: maUUIDp(12)})
		},
	} {
		r := maRisultatoACME()
		cambia(r)
		if f := maFonteP1(r); !f.ConfermaAmmessa || f.Gesto != GestoConfermaSorgente {
			t.Errorf("%s: la conferma di S8 non dipende dal legame: %+v", nome, f)
		}
	}

	// S8 valida sullo STEP della fonte: sorgente confermata, niente seconda conferma, nessun legame storico
	r = maRisultatoACME()
	il := time.Date(2026, 10, 19, 21, 0, 0, 0, time.UTC)
	sha := r.Thread.Documenti[0].Sha256
	r.V1.SorgentiS8 = map[uuid.UUID]SorgenteS8{pid: {AllegatoID: a31, Sha256: sha,
		Stato: confermabom.StatoSorgente{Valida: true, Documento: doc41, ConfermataIl: il, ConfermataDa: maUUID(601)}}}
	f = maFonteP1(r)
	if !f.SorgenteConfermata || f.ConfermaAmmessa || f.Gesto != "" || f.Motivo != MotivoFonteGiaConfermata || f.LegameStorico != nil {
		t.Errorf("S8 valida: %+v", f)
	}
	if s := f.SorgenteS8; s == nil || !s.Valida || s.Motivo != "" || s.DocumentoID == nil || *s.DocumentoID != doc41 || s.ConfermataIl == nil || !s.ConfermataIl.Equal(il) {
		t.Errorf("sorgente_s8: %+v", f.SorgenteS8)
	}

	// S8 revocata: non vale per R5, S8 è di nuovo ammessa, e lo STEP storico non torna (il prodotto ha eventi di S8)
	r.V1.SorgentiS8[pid] = SorgenteS8{AllegatoID: a31, Sha256: sha, Stato: confermabom.StatoSorgente{Motivo: confermabom.SorgenteRevocata, Documento: doc41}}
	f = maFonteP1(r)
	if f.SorgenteConfermata || !f.ConfermaAmmessa || f.SorgenteS8 == nil || f.SorgenteS8.Valida || f.SorgenteS8.Motivo != confermabom.SorgenteRevocata || f.LegameStorico != nil {
		t.Errorf("S8 revocata: %+v %+v", f, f.SorgenteS8)
	}

	// due documenti cad_3d STEP correnti del prodotto: lo STEP storico non è univoco
	r = maRisultatoACME()
	r.Thread.Documenti = append(r.Thread.Documenti, fotorfq.DocumentoConfermato{ID: maUUID(49), ComponenteID: &pid, Tipo: tipoDocumentoCad3D, Estensione: "step",
		Sha256: maSha(49)})
	if f := maFonteP1(r); f.LegameStorico != nil {
		t.Errorf("due STEP del prodotto: nessun legame storico, %+v", f.LegameStorico)
	}
	// uno STEP sostituito non conta; un cad_3d che non è uno STEP non conta
	r.Thread.Documenti[len(r.Thread.Documenti)-1].SostituitoDa = maUUIDp(41)
	r.Thread.Documenti = append(r.Thread.Documenti, fotorfq.DocumentoConfermato{ID: maUUID(48), ComponenteID: &pid, Tipo: tipoDocumentoCad3D, Estensione: "igs",
		NomeFile: "x.igs", Sha256: maSha(48)})
	if f := maFonteP1(r); f.LegameStorico == nil || f.LegameStorico.DocumentoID != doc41 {
		t.Errorf("con un STEP sostituito e un IGES: il legame storico resta il 41, %+v", f.LegameStorico)
	}

	// lo STEP è già il documento di un altro componente (e non del prodotto): S8 lo rifiuterebbe
	r = maRisultatoACME()
	altro := maUUID(12)
	r.Thread.Documenti[0].ComponenteID = &altro
	if f := maFonteP1(r); f.ConfermaAmmessa || f.Motivo != MotivoFonteDocumentoAltrove {
		t.Errorf("STEP documento di un altro componente: %+v", f)
	}
}

func TestV1D39LaRevocaDiS8NonTocca(t *testing.T) {
	doc, altro, prod := maUUID(501), maUUID(502), maUUID(10)
	t0 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	ev := func(n int64, azione string, d uuid.UUID, conChiave bool) db.ListEventiFonteDelThreadRow {
		return db.ListEventiFonteDelThreadRow{EventoID: n, ProdottoID: prod, Azione: azione, DocumentoID: uuid.NullUUID{UUID: d, Valid: true},
			AutoreID: maUUID(601), AutoreNome: "Anna", Quando: t0.Add(time.Duration(n) * time.Hour), ConChiave: conChiave}
	}
	r := &Risultato{Thread: &fotorfq.Thread{}}
	f := func() *FonteDelProdotto {
		d := doc
		x := &FonteDelProdotto{DocumentoID: &d, Confermata: true, Autorizzata: true}
		datiDellaFonte(r, prod, x)
		return x
	}
	// la revoca della sorgente di S8 sullo stesso documento (con la chiave): l'autorizzazione della working resta
	r.V1.EventiFonte = []db.ListEventiFonteDelThreadRow{ev(1, azioneAutorizzazione, doc, false), ev(2, azioneRevoca, doc, true)}
	if x := f(); x.AutorizzataIl == nil || x.AutorizzataMotivo != "" {
		t.Errorf("la revoca di S8 ha tolto l'autorizzazione: %+v", x)
	}
	// la revoca legacy di un altro documento: l'autorizzazione resta (prima ogni revoca del prodotto la toglieva)
	r.V1.EventiFonte = []db.ListEventiFonteDelThreadRow{ev(1, azioneAutorizzazione, doc, false), ev(2, azioneRevoca, altro, false)}
	if x := f(); x.AutorizzataIl == nil || x.AutorizzataMotivo != "" {
		t.Errorf("la revoca di un altro documento ha tolto l'autorizzazione: %+v", x)
	}
	// la revoca legacy dello stesso documento: l'autorizzazione non ha più un evento valido
	r.V1.EventiFonte = []db.ListEventiFonteDelThreadRow{ev(1, azioneAutorizzazione, doc, false), ev(2, azioneRevoca, doc, false)}
	if x := f(); x.AutorizzataIl != nil || x.AutorizzataMotivo != AutorizzazioneNonRegistrata {
		t.Errorf("la revoca legacy dello stesso documento: %+v", x)
	}
}
