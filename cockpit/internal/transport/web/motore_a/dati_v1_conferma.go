package motore_a

// dati_v1_conferma.go: le aggiunte dell'ondata C di V1 al modello di /dati (emendamento U2 §2.4, §5.3, §5.4; U2 §12,
// corsia motore): la data e l'autore della fonte e conferma_corrente (l'ultima conferma formato 3 del prodotto, la sua
// validità, la verifica di adesso e la proposta aggiornata). Le letture stanno nello snapshot della fotografia
// (leggiFonteEConferme, dentro leggiV1); la composizione è pura.
//
// La verifica di adesso (prima-istanza-1, U2 §5.4) è una funzione pura del core che calcola la persistenza (confermabom),
// la stessa di R4 e della S di R5: finché non c'è, verifica_adesso è «non calcolata» con il motivo
// prima_istanza_non_disponibile, mai «sì» (D-18 [U]: con il motore che non risponde «non calcolata adesso», mai «sì») [T].

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

// Persona: chi ha fatto un atto, con il nome (U2 §2.4: {utente_id, nome}).
type Persona struct {
	UtenteID uuid.UUID `json:"utente_id"`
	Nome     string    `json:"nome"`
}

// ConfermaUltima: l'ultima conferma di un prodotto come la legge /dati, con la validità (confermabom: l'ultima, e il
// contatore oppure la firma del perimetro di adesso), il registro delle decisioni e la mappa (per l'adattatore della
// conferma della categoria).
type ConfermaUltima struct {
	ConfermaID     uuid.UUID
	ProdottoID     uuid.UUID
	Numero         int32
	Formato        int16
	ImportazioneID *uuid.UUID
	ConfermataIl   time.Time
	ConfermataDa   Persona
	FonteAllegato  *uuid.UUID
	FonteSha256    string
	FonteGruppo    string
	Criterio       string
	Decisioni      []byte
	Mappa          []byte
	Valida         bool
}

// I motivi della fonte e di conferma_corrente.
const (
	AutorizzazioneNonRegistrata   = "non_registrato"
	StatoVerificaNonCalcolata     = "non_calcolata"
	MotivoPrimaIstanzaNonPresente = "prima_istanza_non_disponibile"
	MotivoGruppoCorrenteAssente   = "gruppo_corrente_assente"
	MotivoRegistroNonImportato    = "registro_non_importato"
	MotivoConfermaSenzaRegistro   = "conferma_senza_registro"
	azioneConfermaDocumento       = "conferma_documento"
	azioneAutorizzazione          = "autorizzazione"
	azioneRevoca                  = "revoca"
	azioneSostituzione            = "sostituzione"
	formatoConfermaV1             = 3
)

// ConfermaCorrente: l'ultima conferma formato 3 del prodotto (U2 §5.3, /dati.conferma_corrente): id e numero, se vale
// ancora, chi e quando, la verifica di adesso e, se la proposta del gruppo corrente è cambiata rispetto al registro della
// conferma, la proposta aggiornata con le differenze nelle sei classi di C-04. Assente per un prodotto senza conferme o
// con l'ultima conferma formato 1 o 2 (per loro vale bom.verificata dell'esito, U2 §5.4). Senza confronto possibile
// (nessun gruppo corrente, il registro di adesso non si importa, la conferma senza registro) proposta_non_confrontata dice
// perché: mai un «nessuna differenza» falso. Il rifiuto della proposta aggiornata (R15, tabella della 0023) non si legge
// ancora: rifiutata non c'è.
type ConfermaCorrente struct {
	ConfermaID             uuid.UUID                       `json:"conferma_id"`
	Numero                 int32                           `json:"numero"`
	Valida                 bool                            `json:"valida"`
	ConfermataIl           time.Time                       `json:"confermata_il"`
	ConfermataDa           Persona                         `json:"confermata_da"`
	VerificaAdesso         VerificaAdesso                  `json:"verifica_adesso"`
	PropostaAggiornata     *propostabom.PropostaAggiornata `json:"proposta_aggiornata,omitempty"`
	PropostaNonConfrontata string                          `json:"proposta_non_confrontata,omitempty"`
}

// VerificaAdesso: la verifica in prima istanza calcolata adesso (U2 §5.4, `/dati` nella tabella «Dove si calcola»):
// stato calcolata | non_calcolata, verificata solo con calcolata, il criterio della conferma, i motivi e le variazioni.
type VerificaAdesso struct {
	Stato      string   `json:"stato"`
	Verificata *bool    `json:"verificata,omitempty"`
	Criterio   string   `json:"criterio,omitempty"`
	Motivi     []string `json:"motivi"`
	Variazioni []string `json:"variazioni"`
}

// leggiFonteEConferme: le letture dell'ondata C nello snapshot: gli eventi della fonte con gli autori, i nomi di chi ha
// confermato i documenti 3D del thread, l'ultima conferma di ogni prodotto con la validità (lo stato della RFQ di
// confermabom, letto qui) e i registri delle ultime conferme formato 3, riletti con le loro colonne.
func leggiFonteEConferme(ctx context.Context, q *db.Queries, thread uuid.UUID, f fotorfq.Fotografia, out *DatiV1) error {
	ev, err := q.ListEventiFonteDelThread(ctx, thread)
	if err != nil {
		return fmt.Errorf("%w: gli eventi della fonte non si leggono: %w", ErrLettura, err)
	}
	out.EventiFonte = ev
	// direttiva 39 §2: l'ultima conferma di S8 di ogni prodotto (la chiave del passo 2, R39-09) e la sua validità per R5,
	// con la funzione del gate (confermabom.SorgenteValida, sola lettura, sulla stessa transazione)
	ultimaS8 := map[uuid.UUID]db.ListEventiFonteDelThreadRow{}
	for _, e := range ev {
		if e.Azione == azioneConfermaDocumento && e.ConChiave {
			ultimaS8[e.ProdottoID] = e
		}
	}
	for p, e := range ultimaS8 {
		st, err := confermabom.SorgenteValida(ctx, q, thread, p, e.Sha256)
		if err != nil {
			return fmt.Errorf("%w: la sorgente confermata da S8 non si legge: %w", ErrLettura, err)
		}
		out.SorgentiS8[p] = SorgenteS8{AllegatoID: e.AllegatoID, Sha256: strings.TrimSpace(e.Sha256), Stato: st}
	}
	var utenti []uuid.UUID
	visti := map[uuid.UUID]bool{}
	for _, t := range f.Thread {
		if t.ID != thread {
			continue
		}
		for _, d := range t.Documenti {
			if !visti[d.ConfermatoDa] {
				visti[d.ConfermatoDa] = true
				utenti = append(utenti, d.ConfermatoDa)
			}
		}
	}
	if len(utenti) > 0 {
		nn, err := q.ListNomiUtenti(ctx, utenti)
		if err != nil {
			return fmt.Errorf("%w: i nomi degli utenti non si leggono: %w", ErrLettura, err)
		}
		for _, n := range nn {
			out.NomiUtenti[n.UtenteID] = n.Nome
		}
	}
	cc, err := q.ListUltimeConfermeDelThread(ctx, thread)
	if err != nil {
		return fmt.Errorf("%w: le ultime conferme non si leggono: %w", ErrLettura, err)
	}
	if len(cc) == 0 {
		return nil
	}
	stato, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return fmt.Errorf("%w: lo stato della RFQ non si legge: %w", ErrLettura, err)
	}
	var registri []uuid.UUID
	for _, c := range cc {
		u := ConfermaUltima{ConfermaID: c.ConfermaID, ProdottoID: c.ProdottoID, Numero: c.Numero, Formato: c.Formato,
			ConfermataIl: c.ConfermatoIl, ConfermataDa: Persona{UtenteID: c.ConfermatoDa, Nome: c.ConfermatoDaNome},
			FonteSha256: strings.TrimSpace(c.FonteSha256.String), FonteGruppo: c.FonteGruppo.String, Criterio: c.CriterioVerifica,
			Decisioni: c.Decisioni, Mappa: c.Mappa}
		if c.ImportazioneID.Valid {
			id := c.ImportazioneID.UUID
			u.ImportazioneID = &id
			if c.Formato == formatoConfermaV1 {
				registri = append(registri, id)
			}
		}
		if c.FonteAllegatoID.Valid {
			a := c.FonteAllegatoID.UUID
			u.FonteAllegato = &a
		}
		if t, v, ok, err := stato.ValiditaUltima(c.ProdottoID); err == nil && ok && t.ID == c.ConfermaID {
			u.Valida = v.Valida
		}
		out.Conferme[c.ProdottoID] = u
	}
	if len(registri) == 0 {
		return nil
	}
	rr, err := q.ListImportazioniPerID(ctx, db.ListImportazioniPerIDParams{ThreadID: thread, Importazioni: registri})
	if err != nil {
		return fmt.Errorf("%w: i registri delle conferme non si leggono: %w", ErrLettura, err)
	}
	for _, r := range rr {
		t := importazione.Testata{ImportazioneID: r.ImportazioneID, ThreadID: r.ThreadID, ProdottoID: r.ProdottoID,
			Impronta: strings.TrimSpace(r.Impronta), ImprontaContenuto: strings.TrimSpace(r.ImprontaContenuto),
			ImprontaEvidenze: strings.TrimSpace(r.ImprontaEvidenze), ImprontaRegole: strings.TrimSpace(r.ImprontaRegole),
			FormatoRegistro: int(r.FormatoRegistro), VersioneImportazione: r.VersioneImportazione, Gruppo: r.Gruppo,
			FonteSha256: strings.TrimSpace(r.FonteSha256.String), FonteRadice: r.FonteRadice.String, AllegatiCitati: r.AllegatiCitati,
			CreataDa: r.CreataDa, CreataIl: r.CreataIl}
		if r.FonteAllegatoID.Valid {
			a := r.FonteAllegatoID.UUID
			t.FonteAllegatoID = &a
		}
		reg, err := importazione.Rileggi(t, r.Contenuto)
		if err != nil {
			// un registro salvato che non si rilegge è un guasto dei dati: la conferma resta, il confronto no
			continue
		}
		out.Registri[r.ImportazioneID] = reg.Contenuto
	}
	return nil
}

// datiDellaFonte: la data e l'autore della conferma e dell'autorizzazione della fonte del prodotto (U2 §2.4), dagli
// eventi della fonte; per una conferma senza evento (prima di V1, o con l'ultima conferma seguita da una sostituzione)
// dal documento; un'autorizzazione senza evento dice non_registrato.
func datiDellaFonte(r *Risultato, prodotto uuid.UUID, f *FonteDelProdotto) {
	if f == nil || f.DocumentoID == nil {
		return
	}
	doc := *f.DocumentoID
	var conferma, autorizzazione *db.ListEventiFonteDelThreadRow
	for i := range r.V1.EventiFonte {
		e := &r.V1.EventiFonte[i]
		if e.ProdottoID != prodotto {
			continue
		}
		perDoc := e.DocumentoID.Valid && e.DocumentoID.UUID == doc
		switch e.Azione {
		case azioneConfermaDocumento:
			if perDoc {
				conferma = e
			}
		case azioneAutorizzazione:
			if perDoc {
				autorizzazione = e
			}
		case azioneSostituzione:
			// «non seguita da una sostituzione» (U2 §2.4): dopo la sostituzione vale solo una conferma nuova
			conferma = nil
		case azioneRevoca:
			// direttiva 39 (R39-10, richiesta della corsia persistenza): solo la revoca dell'autorizzazione di questo
			// documento la toglie; la revoca della sorgente della bozza non tocca la working né la sua autorizzazione.
			// Dalla variante 20261019c della 0023 la revoca di S8 ha l'azione dedicata revoca_sorgente, che
			// ListEventiFonteDelThread non legge (la validità della sorgente la dice confermabom.SorgenteValida in
			// sorgente_s8): «revoca» è solo la revoca legacy dell'autorizzazione. La condizione sulla chiave d'intento
			// resta per prudenza [T-39]: un evento «revoca» con la chiave non è la revoca dell'autorizzazione.
			// Prima ogni revoca del prodotto azzerava l'autorizzazione.
			if perDoc && !e.ConChiave {
				autorizzazione = nil
			}
		}
	}
	if conferma != nil {
		il := conferma.Quando
		f.ConfermataIl, f.ConfermataDa = &il, &Persona{UtenteID: conferma.AutoreID, Nome: conferma.AutoreNome}
	} else if r.Thread != nil {
		for _, d := range r.Thread.Documenti {
			if d.ID == doc {
				il := d.ConfermatoIl
				f.ConfermataIl, f.ConfermataDa = &il, &Persona{UtenteID: d.ConfermatoDa, Nome: r.V1.NomiUtenti[d.ConfermatoDa]}
			}
		}
	}
	if !f.Autorizzata {
		return
	}
	if autorizzazione == nil {
		f.AutorizzataMotivo = AutorizzazioneNonRegistrata
		return
	}
	il := autorizzazione.Quando
	f.AutorizzataIl, f.AutorizzataDa = &il, &Persona{UtenteID: autorizzazione.AutoreID, Nome: autorizzazione.AutoreNome}
}

// confermaCorrente: conferma_corrente del prodotto (ConfermaCorrente), nil senza un'ultima conferma formato 3.
func confermaCorrente(r *Risultato, p *Pagina, prodotto uuid.UUID, m *propostabom.ModelloDelProdotto) *ConfermaCorrente {
	c, ok := r.V1.Conferme[prodotto]
	if !ok || c.Formato != formatoConfermaV1 {
		return nil
	}
	out := &ConfermaCorrente{ConfermaID: c.ConfermaID, Numero: c.Numero, Valida: c.Valida, ConfermataIl: c.ConfermataIl,
		ConfermataDa: c.ConfermataDa, VerificaAdesso: VerificaAdesso{Stato: StatoVerificaNonCalcolata, Criterio: c.Criterio,
			Motivi: []string{MotivoPrimaIstanzaNonPresente}, Variazioni: []string{}}}
	if c.ImportazioneID == nil {
		out.PropostaNonConfrontata = MotivoConfermaSenzaRegistro
		return out
	}
	reg, ok := r.V1.Registri[*c.ImportazioneID]
	if !ok {
		out.PropostaNonConfrontata = MotivoConfermaSenzaRegistro
		return out
	}
	var gc propostabom.GruppoCorrente
	trovato := false
	for _, pv := range r.EsitoThread.ProdottiValutati {
		if pv.Rif != p.Prodotto || reg.Proposta.Fonte.AllegatoID == "" {
			continue
		}
		gc = propostabom.GruppoCorrenteDelProdotto(pv, r.EsitoThread.Ancoraggi.Strutture, r.V1.NonAttivi,
			propostabom.SorgenteBozza{AllegatoID: uuid.MustParse(reg.Proposta.Fonte.AllegatoID), Sha256: reg.Proposta.Fonte.Sha256,
				Radice: reg.Gruppo.Radice, Gruppo: reg.Gruppo.Chiave})
		trovato = true
	}
	if !trovato || gc.Chiave == "" {
		out.PropostaNonConfrontata = MotivoGruppoCorrenteAssente
		return out
	}
	adesso, ok := m.Registri[gc.Chiave]
	if !ok {
		out.PropostaNonConfrontata = MotivoRegistroNonImportato
		return out
	}
	out.PropostaAggiornata = propostabom.PropostaAggiornataDi(reg, adesso)
	return out
}

// categorieDelThread: le conferme della categoria del thread per la valutazione (propostabom.ConfermeCategoria) dalle
// ultime conferme dei prodotti lette nello snapshot; nil senza conferme (l'esito è quello di prima).
func categorieDelThread(thread uuid.UUID, v DatiV1) map[uuid.UUID][]valutazione.ConfermaCategoria {
	var cc []propostabom.ConfermaDelProdotto
	for _, c := range v.Conferme {
		cc = append(cc, propostabom.ConfermaDelProdotto{Valida: c.Valida, ConfermataDa: c.ConfermataDa.UtenteID, ConfermataIl: c.ConfermataIl,
			Decisioni: c.Decisioni, Mappa: c.Mappa})
	}
	k := propostabom.ConfermeCategoria(cc)
	if len(k) == 0 {
		return nil
	}
	return map[uuid.UUID][]valutazione.ConfermaCategoria{thread: k}
}
