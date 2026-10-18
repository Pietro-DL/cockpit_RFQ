package motore_a

// dati_v1.go: le aggiunte di V1 al modello della pagina (/motore-a/dati; emendamento di V1 §9.9, §6.4.1; PE-01, PE-10,
// PE-15, PE-16, PE-28 dell'interfaccia, accolte nelle decisioni dell'ondata A; piano della 0023 §14 passo 3).
//
// Le letture nuove stanno nello stesso snapshot REPEATABLE READ READ ONLY della fotografia (leggiV1): le occorrenze non
// attive (ListFileRfqNonAttivi), le bozze aperte formato 3 con le colonne del loro registro (ListSorgentiBozzeAperte, mai
// il contenuto), lo stato di F8 (GetStatoF8DelThread) e l'identità certificata dei componenti (ListIdentitaRegistrate).
// La composizione (componiV1) è pura: i gruppi del prodotto con l'impronta_contenuto di un'importazione fatta adesso, i
// nodi con la completezza e le diagnostiche correnti e l'identità registrata (propostabom.Modello, lo stesso adattatore
// di R2 e di S6), la fonte del prodotto, lo STEP secondario, il motivo della mancanza di gruppi, la bozza del prodotto con
// il gruppo corrente della sorgente (propostabom.GruppoCorrenteDelProdotto, sui gruppi senza le occorrenze non attive).
// Nessuna scrittura: è una GET (D-18).

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/platform/db"
)

// DatiV1: ciò che il modello di V1 legge dal database, nello snapshot della fotografia.
//   - NonAttivi: le occorrenze in «Altri file» o scartate (il ciclo dei file), per allegato con lo stato.
//   - Bozze: la bozza aperta formato 3 di ogni prodotto, con le colonne del registro della versione.
//   - F8: lo stato del rismistamento della RFQ (F8Errore, F8InAttesa, F8Eseguito, o "" senza righe).
//   - Certificazioni: l'identità certificata dall'ultima conferma formato 3 di ogni componente che ne ha una.
type DatiV1 struct {
	NonAttivi      propostabom.NonAttivi
	Bozze          map[uuid.UUID]BozzaAperta
	F8             string
	Certificazioni map[uuid.UUID]propostabom.Certificazione
}

// BozzaAperta: la testata di una bozza aperta formato 3 e le colonne del registro della sua versione.
type BozzaAperta struct {
	BozzaID        uuid.UUID
	Versione       int32
	FonteAllegato  *uuid.UUID
	FonteSha256    string
	FonteGruppo    string
	ConRegistro    bool
	RegistroGruppo string
	FonteRadice    string
}

// Gli stati di F8 della RFQ nel modello (f8_stato, 0022): errore se una riga è in errore, in_attesa se una è in attesa,
// eseguito se tutte sono eseguite; "" senza righe (F8 mai previsto).
const (
	F8Errore   = "errore"
	F8InAttesa = "in_attesa"
	F8Eseguito = "eseguito"
)

// leggiV1: le letture di V1 sulla transazione dello snapshot, dopo la fotografia (che dà i componenti del thread).
func leggiV1(ctx context.Context, q *db.Queries, thread uuid.UUID, f fotorfq.Fotografia) (DatiV1, error) {
	out := DatiV1{NonAttivi: propostabom.NonAttivi{}, Bozze: map[uuid.UUID]BozzaAperta{}, Certificazioni: map[uuid.UUID]propostabom.Certificazione{}}
	na, err := q.ListFileRfqNonAttivi(ctx, thread)
	if err != nil {
		return out, fmt.Errorf("%w: le occorrenze non attive non si leggono: %w", ErrLettura, err)
	}
	for _, r := range na {
		out.NonAttivi[r.AllegatoID] = r.Stato
	}
	bb, err := q.ListSorgentiBozzeAperte(ctx, thread)
	if err != nil {
		return out, fmt.Errorf("%w: le bozze aperte non si leggono: %w", ErrLettura, err)
	}
	for _, r := range bb {
		if _, ok := out.Bozze[r.ProdottoID]; ok {
			continue // una bozza aperta per prodotto (il trigger della 0022): la prima, in ordine di bozza
		}
		b := BozzaAperta{BozzaID: r.BozzaID, Versione: r.Versione, FonteSha256: r.FonteSha256.String, FonteGruppo: r.FonteGruppo.String,
			ConRegistro: r.ImportazioneID.Valid, RegistroGruppo: r.RegistroGruppo.String, FonteRadice: r.RegistroFonteRadice.String}
		if r.FonteAllegatoID.Valid {
			a := r.FonteAllegatoID.UUID
			b.FonteAllegato = &a
		}
		out.Bozze[r.ProdottoID] = b
	}
	f8, err := q.GetStatoF8DelThread(ctx, thread)
	if err != nil {
		return out, fmt.Errorf("%w: lo stato di F8 non si legge: %w", ErrLettura, err)
	}
	switch {
	case f8.InErrore > 0:
		out.F8 = F8Errore
	case f8.InAttesa > 0:
		out.F8 = F8InAttesa
	case f8.Totale > 0:
		out.F8 = F8Eseguito
	}
	var componenti []uuid.UUID
	for _, t := range f.Thread {
		if t.ID != thread {
			continue
		}
		for _, c := range t.Componenti {
			componenti = append(componenti, c.ID)
		}
	}
	if len(componenti) > 0 {
		cc, err := q.ListIdentitaRegistrate(ctx, db.ListIdentitaRegistrateParams{ThreadID: thread, Componenti: componenti})
		if err != nil {
			return out, fmt.Errorf("%w: le identità certificate non si leggono: %w", ErrLettura, err)
		}
		for _, r := range cc {
			out.Certificazioni[r.ComponenteID] = propostabom.Certificazione{Codice: r.Codice, Emittente: r.Emittente.String,
				EmittentePresunto: r.EmittentePresunto.Bool, Namespace: r.Namespace.String}
		}
	}
	return out, nil
}

// FonteDelProdotto: la fonte del prodotto nel modello (PE-01; EM §9.9 «per la fonte: conferma_ammessa con il motivo, e
// lo STEP secondario»): lo STEP che la pagina guarda per gli atti della fonte, con la proposta da confermare («Conferma
// la fonte…», una voce), il documento e la radice per l'anteprima dell'autorizzazione («Autorizza»), se la conferma è
// ammessa e perché no, se la fonte è confermata e autorizzata, e lo STEP secondario (PE-15).
//
// Lo STEP è, in quest'ordine: la sorgente della bozza aperta del prodotto; lo STEP autorizzato (il documento di
// step_strutturale_id); la candidata compatibile del prodotto, se è una sola; il primo gruppo con uno STEP. Senza STEP
// la conferma non è ammessa, con il motivo. Nel JSON i soli nomi di PE-01 (A-28): l'allegato resta per chi chiama.
type FonteDelProdotto struct {
	Voce            *uuid.UUID                  `json:"voce,omitempty"`
	AllegatoID      *uuid.UUID                  `json:"-"`
	DocumentoID     *uuid.UUID                  `json:"documento_id,omitempty"`
	Sha256          string                      `json:"sha256,omitempty"`
	ConfermaAmmessa bool                        `json:"conferma_ammessa"`
	Motivo          string                      `json:"motivo,omitempty"`
	Confermata      bool                        `json:"confermata"`
	Autorizzata     bool                        `json:"autorizzata"`
	Sorgente        string                      `json:"sorgente,omitempty"`
	StepSecondario  *propostabom.StepSecondario `json:"step_secondario,omitempty"`
}

// I motivi di una conferma della fonte non ammessa. MotivoFonteSenzaComponente: il prodotto non ha un componente della
// RFQ (lo stesso testo del motivo della fonte prodotto_senza_componente); MotivoFonteSTEPNonAttivo: lo STEP del prodotto
// sta in «Altri file» o è scartato (il modello lo lascia fuori dai gruppi, EM §9.9), e il rimedio è «Riattiva».
const (
	MotivoFonteSenzaSTEP       = "il prodotto non ha uno STEP fra i gruppi del motore"
	MotivoFonteSenzaComponente = "il prodotto non ha un componente della RFQ"
	MotivoFonteSTEPNonAttivo   = propostabom.TestoStepNonAttivo
	MotivoFonteGiaConfermata   = "la fonte è già confermata"
	MotivoFonteSenzaProposta   = "lo STEP non ha una proposta aperta da confermare"
	MotivoFonteNonCad3D        = "la proposta dello STEP non è un 3D"
	MotivoFonteNonAssociata    = "lo STEP non è associato al prodotto: associalo dalla Sorgente"
	tipoDocumentoCad3D         = "cad_3d"
	statoPropostaApertaModello = "aperta"
)

// BozzaDelProdotto: la bozza aperta formato 3 del prodotto con un registro e il gruppo corrente della sorgente (EM §6.4.1,
// §9.9): con assente il motivo è nessun_gruppo o occorrenza_non_attiva, perché una bozza con un registro ha sempre la
// radice. Per una bozza senza registro il gruppo corrente lo dicono R1 ?confronto=1 e riallinea_proposta: qui non c'è.
type BozzaDelProdotto struct {
	BozzaID        uuid.UUID                  `json:"bozza_id"`
	Versione       int32                      `json:"versione"`
	GruppoCorrente propostabom.GruppoCorrente `json:"gruppo_corrente"`
}

// componiV1: le aggiunte di V1 alla pagina del prodotto scelto.
func componiV1(p *Pagina, r *Risultato) {
	et := r.EsitoThread
	var pid *uuid.UUID
	for i := range et.ProdottiValutati {
		if et.ProdottiValutati[i].Rif == p.Prodotto {
			pid = et.ProdottiValutati[i].ComponenteID
		}
	}
	componenti := map[uuid.UUID]propostabom.ComponenteDellaWorking{}
	for _, c := range r.Thread.Componenti {
		k := propostabom.ComponenteDellaWorking{Codice: c.Codice, Archiviato: c.ArchiviatoIl != nil}
		if c.Rev != nil {
			k.Rev = *c.Rev
		}
		componenti[c.ID] = k
	}
	m := propostabom.Modello(propostabom.IngressoModello{Esito: &r.Esito, Thread: et, Regole: r.Regole, Prodotto: p.Prodotto,
		NonAttivi: r.V1.NonAttivi, Componenti: componenti, Certificazioni: r.V1.Certificazioni})
	p.F8Stato = r.V1.F8
	if m == nil {
		p.Gruppi = []propostabom.GruppoDelModello{}
		return
	}
	p.Gruppi, p.MotivoSenzaGruppi = m.Gruppi, m.MotivoSenzaGruppi
	if pid != nil {
		if b, ok := r.V1.Bozze[*pid]; ok && b.ConRegistro && b.FonteAllegato != nil {
			for i := range et.ProdottiValutati {
				if pv := et.ProdottiValutati[i]; pv.Rif == p.Prodotto {
					gc := propostabom.GruppoCorrenteDelProdotto(pv, et.Ancoraggi.Strutture, r.V1.NonAttivi,
						propostabom.SorgenteBozza{AllegatoID: *b.FonteAllegato, Sha256: b.FonteSha256, Radice: b.FonteRadice, Gruppo: b.FonteGruppo})
					p.BozzaDelProdotto = &BozzaDelProdotto{BozzaID: b.BozzaID, Versione: b.Versione, GruppoCorrente: gc}
				}
			}
		}
	}
	p.Fonte = fonteDelProdotto(r, p, pid, m)
}

// fonteDelProdotto: la fonte del prodotto (FonteDelProdotto). Uno STEP in «Altri file» o scartato non si conferma: con lo
// STEP scelto non attivo, o senza STEP perché il modello ha lasciato fuori quelli non attivi, il motivo lo dice.
func fonteDelProdotto(r *Risultato, p *Pagina, pid *uuid.UUID, m *propostabom.ModelloDelProdotto) *FonteDelProdotto {
	out := &FonteDelProdotto{StepSecondario: m.StepSecondario}
	if pid == nil {
		out.Motivo = MotivoFonteSenzaComponente
		return out
	}
	t := r.Thread
	var comp *fotorfq.Componente
	for i := range t.Componenti {
		if t.Componenti[i].ID == *pid {
			comp = &t.Componenti[i]
		}
	}
	var allegato *uuid.UUID
	if b, ok := r.V1.Bozze[*pid]; ok && b.FonteAllegato != nil {
		allegato = b.FonteAllegato
	}
	if allegato == nil && comp != nil && comp.StepStrutturaleID != nil {
		for i := range t.Documenti {
			if d := &t.Documenti[i]; d.ID == *comp.StepStrutturaleID && len(d.Allegati) > 0 {
				a := d.Allegati[0]
				allegato = &a
			}
		}
	}
	if allegato == nil {
		compatibili := 0
		var primo, compatibile *uuid.UUID
		for _, g := range m.Gruppi {
			if g.AllegatoID == nil || g.Tipo == propostabom.GruppoSenzaSTEP {
				continue
			}
			if primo == nil {
				primo = g.AllegatoID
			}
			if g.Tipo == propostabom.GruppoCandidata && g.Compatibilita == propostabom.CompatibilitaCompatibile {
				compatibili++
				compatibile = g.AllegatoID
			}
		}
		allegato = primo
		if compatibili == 1 {
			allegato = compatibile
		}
	}
	if allegato == nil {
		out.Motivo = MotivoFonteSenzaSTEP
		if len(m.StepNonAttivi) > 0 {
			out.Motivo = MotivoFonteSTEPNonAttivo
		}
		return out
	}
	a := *allegato
	out.AllegatoID = &a
	_, nonAttivo := r.V1.NonAttivi[a]
	for _, f := range r.EsitoThread.File {
		if f.AllegatoID == a {
			out.Sha256 = f.Documento.Fonte.RiferimentoFatti.Sha256
		}
	}
	for i := range t.Documenti {
		d := &t.Documenti[i]
		if d.Tipo != tipoDocumentoCad3D || d.ComponenteID == nil || *d.ComponenteID != *pid || d.SostituitoDa != nil {
			continue
		}
		dentro := out.Sha256 != "" && d.Sha256 == out.Sha256
		for _, x := range d.Allegati {
			dentro = dentro || x == a
		}
		if dentro {
			id := d.ID
			out.DocumentoID, out.Confermata = &id, true
			out.Autorizzata = comp != nil && comp.StepStrutturaleID != nil && *comp.StepStrutturaleID == d.ID
			break
		}
	}
	if p.Indice != nil {
		if au := p.Indice.AutorizzazioneDi(p.Prodotto, a); au != nil {
			out.Sorgente = au.Sorgente
		}
		out.Voce = p.Indice.PropostaAperta(a)
	}
	switch {
	case out.Confermata:
		out.Motivo = MotivoFonteGiaConfermata
	case nonAttivo:
		out.Motivo = MotivoFonteSTEPNonAttivo
	case out.Voce == nil:
		out.Motivo = MotivoFonteSenzaProposta
	default:
		for _, pr := range t.Proposte {
			if pr.ID != *out.Voce {
				continue
			}
			switch {
			case pr.Tipo != tipoDocumentoCad3D:
				out.Motivo = MotivoFonteNonCad3D
			case pr.ComponenteID == nil || *pr.ComponenteID != *pid:
				out.Motivo = MotivoFonteNonAssociata
			case pr.Stato == statoPropostaApertaModello:
				out.ConfermaAmmessa = true
			}
		}
	}
	return out
}
