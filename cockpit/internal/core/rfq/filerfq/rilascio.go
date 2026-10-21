package filerfq

import (
	"context"
	"errors"
	"os"
	"syscall"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// I motivi della rimozione dal NAS che non si ritentano da soli (U2 §3.5, §3.7, U2-P42, U2-P43): la scrittura spenta
// aspetta «Riprova la rimozione dalla cartella ALTRO» (niente parte da solo quando la capacita' si accende, come
// AllineaCoda); radice cambiata, file fuori dalla cartella e nome ripreso li chiude una bonifica dell'amministratore.
// Il custode non li riprende (ListIdConservazioniDaRiprendere).
var motiviFermi = map[string]bool{nas.MotivoScritturaSpenta: true, nas.MotivoRadiceCambiata: true, nas.MotivoFuoriCartella: true,
	nas.MotivoNomeRipreso: true}

// RilasciaCopia e' un passo del rilascio di una copia degli Altri file (EM §10.5, «Il rilascio di una copia»; U2 §3.3
// passi 6-7, §3.7, §3.8). Il protocollo della rimozione dal NAS non tiene mai una transazione aperta durante l'I/O di
// rete e non sostituisce mai un file:
//
//  1. transazione breve: la riga da_rilasciare FOR UPDATE (il custode SKIP LOCKED) e i controlli:
//     - la copia in volo (una riga chiusa mentre era in copia, senza la prova di creazione) aspetta che la copia
//     registri o che il battito sia fermo da 15 minuti; intanto niente si scrive;
//     - base B di U2-Q01 (V1-Q25, provvisoria [T]): sul NAS il cockpit cancella solo per lo scarto. Per ogni altra causa
//     la riga va a trattenuta, il file resta (trattieni);
//     - la scrittura sul NAS spenta (o nessun posto per gli Altri file su questo server): la riga resta da_rilasciare
//     con scrittura_nas_spenta, e la riprende solo «Riprova la rimozione dalla cartella ALTRO»;
//     - un'altra radice: errore radice_cambiata; nessuna cartella della RFQ fotografata: errore fuori_cartella;
//     - senza la prova di creazione il file non si toglie mai (U2 §3.3 passo 7): la riga si chiude con non_nostro (o
//     gia_assente se al percorso non c'e' niente);
//     - sotto BloccaCartella(thread, «ALTRO»): un documento, uno spostamento pendente o un orfano aperto che nomina quel
//     percorso chiude la riga con percorso_di_un_documento (lo scarto non tocca mai un documento, §3.6 regola 2);
//     - la riga passa a in_rimozione con il token di rimozione: quello della colonna se c'e' gia' (una ripresa ritrova
//     il nome di servizio), altrimenti uno nuovo. COMMIT;
//  2. sul NAS, fuori transazione: Conservatore.Rimuovi (la rinomina esclusiva nel nome di servizio, la verifica dello
//     sha dallo stesso handle, la cancellazione del nome di servizio);
//  3. transazione breve: la riga ancora in_rimozione con lo stesso token → rilasciata con l'esito (rimosso, gia_assente,
//     file_cambiato) e l'evento; oppure di nuovo da_rilasciare con l'esito trovato (in attesa: file_occupato,
//     nas_irraggiungibile, scrittura_nas_spenta; errore: nome_ripreso, radice_cambiata, fuori_cartella), il testo, i
//     tentativi e il prossimo tentativo.
//
// Una riga rimasta in_rimozione (un'esecuzione interrotta fra 1 e 3) la riprende solo il custode, dopo 15 minuti, dal
// passo 2 con il token della colonna.
func (s *Servizio) RilasciaCopia(ctx context.Context, conservazioneID int64, custode bool) (EsitoPasso, error) {
	var c db.FileRfqConservazione
	var token uuid.UUID
	ripresa := false
	var fatto *EsitoPasso
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if custode {
			c, err = q.GetConservazioneSeLibera(ctx, conservazioneID)
		} else {
			c, err = q.GetConservazionePerAggiornare(ctx, conservazioneID)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			fatto = &EsitoPasso{Saltato: true}
			return nil
		}
		if err != nil {
			return err
		}
		switch c.Stato {
		case "in_rimozione":
			// un'esecuzione interrotta fra il passo 1 e il passo 3: la riprende solo il custode, dopo 15 minuti (un'altra
			// esecuzione puo' essere ancora sul NAS), dal passo 2 con il token della colonna
			if !custode || !c.TokenRimozione.Valid || s.adesso().Sub(c.AggiornatoIl) < battitoFermo {
				fatto = &EsitoPasso{Saltato: true, Stato: c.Stato}
				return nil
			}
			token, ripresa = c.TokenRimozione.UUID, true
			return nil
		case "da_rilasciare":
		default:
			fatto = &EsitoPasso{Saltato: true, Stato: c.Stato}
			return nil
		}
		if !c.CreatoDalCockpit && s.adesso().Sub(battitoDi(c)) < battitoFermo {
			// la copia in volo: niente si scrive, si riprova dopo (il battito continua anche sulla riga chiusa)
			fatto = &EsitoPasso{Stato: "da_rilasciare", Motivo: "copia_in_volo"}
			return nil
		}
		if !c.Causa.Valid || c.Causa.String != "scartato" {
			p, err := s.trattieni(ctx, q, c)
			fatto = &p
			return err
		}
		ferma := func(motivo string) error {
			p, err := s.rimanda(ctx, q, c, motivo)
			fatto = &p
			return err
		}
		chiudi := func(motivo string) error {
			if err := s.passoConservazioneConEsito(ctx, q, c, "rilasciata", c.Causa, pgtype.Text{}, motivo); err != nil {
				return err
			}
			fatto = &EsitoPasso{Stato: "rilasciata", Motivo: motivo}
			return nil
		}
		if ok, _ := s.disponibile(); !ok {
			return ferma(nas.MotivoScritturaSpenta)
		}
		if c.RadiceID != s.Conservatore.RadiceID() {
			return ferma(nas.MotivoRadiceCambiata)
		}
		if !c.CartellaThread.Valid {
			return ferma(nas.MotivoFuoriCartella)
		}
		if !c.CreatoDalCockpit {
			st, err := s.Conservatore.Stato(postoDellaRiga(c), false)
			if err == nil && st == nas.CopiaNonTrovata {
				return chiudi(nas.MotivoGiaAssente)
			}
			return chiudi(nas.MotivoNonNostro)
		}
		sotto, err := cartellaAltro(ctx, q)
		if err != nil {
			return err
		}
		if err := documenti.BloccaCartella(ctx, q, c.ThreadID, sotto); err != nil {
			return err
		}
		doc, err := q.PercorsoDiUnDocumento(ctx, db.PercorsoDiUnDocumentoParams{ThreadID: c.ThreadID, Percorso: c.PercorsoRelativo})
		if err != nil {
			return err
		}
		if doc {
			return chiudi(nas.MotivoPercorsoDiUnDocumento)
		}
		token, ripresa = uuid.New(), c.TokenRimozione.Valid
		if ripresa {
			token = c.TokenRimozione.UUID
		}
		n, err := q.AggiornaConservazione(ctx, db.AggiornaConservazioneParams{Stato: "in_rimozione", Causa: c.Causa,
			Tentativi: c.Tentativi, TokenRimozione: uuid.NullUUID{UUID: token, Valid: true}, ConservazioneID: c.ConservazioneID,
			StatoAtteso: "da_rilasciare"})
		if err == nil && n == 0 {
			err = errors.New("copia di conservazione: lo stato e' cambiato durante il passo")
		}
		return err
	})
	if err != nil || fatto != nil {
		if fatto == nil {
			return EsitoPasso{}, err
		}
		return *fatto, err
	}
	// passo 2: sul NAS, senza transazioni aperte
	if s.Agganci != nil && s.Agganci.PrimaDellaRimozioneNAS != nil {
		s.Agganci.PrimaDellaRimozioneNAS(c.ConservazioneID)
	}
	e := s.Conservatore.Rimuovi(postoDellaRiga(c), token, ripresa)
	if e.Esito == nas.RimozioneErrore {
		s.log().Error("ciclo dei file: la copia non si toglie: serve una bonifica", "conservazione", c.ConservazioneID,
			"motivo", e.Motivo)
	}
	// passo 3: l'esito, se la riga e' ancora in rimozione con questo token
	var esito EsitoPasso
	err = s.inTx(context.WithoutCancel(ctx), func(q *db.Queries) error {
		r, err := q.GetConservazionePerAggiornare(ctx, c.ConservazioneID)
		if err != nil {
			return err
		}
		if r.Stato != "in_rimozione" || !r.TokenRimozione.Valid || r.TokenRimozione.UUID != token {
			esito = EsitoPasso{Saltato: true, Stato: r.Stato}
			return nil
		}
		switch e.Esito {
		case nas.RimozioneEseguita, nas.RimozioneNonNecessaria:
			if err := s.passoConservazioneConEsito(ctx, q, r, "rilasciata", r.Causa, pgtype.Text{}, e.Motivo); err != nil {
				return err
			}
			esito = EsitoPasso{Stato: "rilasciata", Motivo: e.Motivo}
			return nil
		}
		esito, err = s.rimanda(ctx, q, r, e.Motivo)
		return err
	})
	return esito, err
}

// rimanda: la riga torna (o resta) da_rilasciare con l'esito della rimozione dal NAS (U2 §3.7) e il suo testo, i
// tentativi + 1 e il prossimo tentativo (che il custode non usa per i motivi fermi).
func (s *Servizio) rimanda(ctx context.Context, q *db.Queries, c db.FileRfqConservazione, motivo string) (EsitoPasso, error) {
	testo := TestoRimozioneNAS(nas.RimozioneInAttesa, motivo)
	if err := s.passoConservazioneConEsito(ctx, q, c, "da_rilasciare", c.Causa, pgtype.Text{String: testo, Valid: true}, motivo); err != nil {
		return EsitoPasso{}, err
	}
	return EsitoPasso{Stato: "da_rilasciare", Motivo: motivo}, nil
}

// trattieni: una copia che esce da Altri file per una causa che non e' lo scarto (base B di U2-Q01, V1-Q25,
// provvisoria [T]; U2 §3.8): il cockpit non la toglie dal NAS.
//   - con la prova di creazione la riga va a trattenuta («copia del cockpit non piu' in Altri file»): tiene la prova,
//     il token, il nome e lo sha, e il file resta; un nuovo «Metti in Altri file» dello stesso contenuto nella stessa
//     RFQ lo riusa ed eredita la prova (eredita_da), e la riga passa a ceduta;
//   - senza la prova (un'interruzione fra la promozione e la registrazione) il file non e' provato del cockpit: resta,
//     e la riga si chiude rilasciata con l'esito non_nostro;
//   - se la copia, sotto la radice di questo server, non c'e' piu' al percorso registrato, la riga si chiude
//     rilasciata con l'esito gia_assente (mai «eliminato»).
//
// Niente si scrive sul NAS. Con la risposta A o C di U2-Q01 le cause indicate tornerebbero al protocollo del §3.7.
func (s *Servizio) trattieni(ctx context.Context, q *db.Queries, c db.FileRfqConservazione) (EsitoPasso, error) {
	chiudi := func(motivo string) (EsitoPasso, error) {
		if err := s.passoConservazioneConEsito(ctx, q, c, "rilasciata", c.Causa, pgtype.Text{}, motivo); err != nil {
			return EsitoPasso{}, err
		}
		return EsitoPasso{Stato: "rilasciata", Motivo: motivo}, nil
	}
	if s.Conservatore != nil && c.RadiceID == s.Conservatore.RadiceID() {
		if st, err := s.Conservatore.Stato(postoDellaRiga(c), false); err == nil && st == nas.CopiaNonTrovata {
			return chiudi(nas.MotivoGiaAssente)
		}
	}
	if !c.CreatoDalCockpit {
		return chiudi(nas.MotivoNonNostro)
	}
	if err := s.passoConservazione(ctx, q, c, "trattenuta", c.Causa, pgtype.Text{}); err != nil {
		return EsitoPasso{}, err
	}
	return EsitoPasso{Stato: "trattenuta"}, nil
}

// TestoRimozioneNAS: il testo della rimozione di una copia degli Altri file (U2 §3.7, la tabella), senza percorsi.
// «Eliminato» solo con rimosso.
func TestoRimozioneNAS(esito, motivo string) string {
	switch motivo {
	case nas.MotivoScritturaSpenta:
		return "la copia in ALTRO sul NAS è ancora da togliere: la scrittura sul NAS è spenta su questo server. Quando sarà accesa, usa «Riprova la rimozione dalla cartella ALTRO»"
	case nas.MotivoNasIrraggiungibil:
		return "la copia in ALTRO sul NAS è ancora da togliere: il NAS non risponde. Si riprova da solo"
	case nas.MotivoFileOccupato:
		return "la copia in ALTRO sul NAS è aperta da un altro programma: si toglie appena si chiude"
	case "copia_in_volo":
		return "la copia verso ALTRO è ancora in corso: si toglie appena finisce"
	case nas.MotivoRadiceCambiata, nas.MotivoFuoriCartella:
		return "il file in ALTRO non è stato tolto: serve una bonifica dell'amministratore"
	case nas.MotivoNomeRipreso:
		return "il file in ALTRO non è stato tolto: il suo nome di prima è stato ripreso da un altro file, e ora ha un nome di servizio nascosto nella stessa cartella. Serve una bonifica dell'amministratore"
	case nas.MotivoRimosso:
		return "file eliminato anche dalla cartella ALTRO della RFQ"
	case nas.MotivoGiaAssente:
		return "la copia non è stata trovata al percorso registrato: la cartella può essere stata spostata o rinominata"
	case nas.MotivoFileCambiato:
		return "il file in ALTRO è stato cambiato da una persona: il cockpit non lo tocca"
	case nas.MotivoNonNostro:
		return "il file in ALTRO non l'ha creato il cockpit: resta dov'è"
	case nas.MotivoPercorsoDiUnDocumento:
		return "il file in ALTRO è un documento confermato: resta dov'è"
	}
	return "la copia in ALTRO è ancora da togliere (" + esito + ")"
}

// passoConservazione scrive un passo di una copia dallo stato atteso e, per gli stati che hanno un evento (rilasciata,
// trattenuta, errore_copia), l'evento del server.
func (s *Servizio) passoConservazione(ctx context.Context, q *db.Queries, c db.FileRfqConservazione, stato string, causa,
	errore pgtype.Text) error {
	return s.passoConservazioneConEsito(ctx, q, c, stato, causa, errore, "")
}

// passoConservazioneConEsito: passoConservazione con il motivo della rimozione dal NAS (U2 §3.7) nella colonna
// esito_rimozione e nell'esito dell'evento. Da in_rimozione o da da_rilasciare verso da_rilasciare i tentativi crescono
// di uno, con il prossimo tentativo.
func (s *Servizio) passoConservazioneConEsito(ctx context.Context, q *db.Queries, c db.FileRfqConservazione, stato string, causa,
	errore pgtype.Text, motivoRimozione string) error {
	p := db.AggiornaConservazioneParams{Stato: stato, Causa: causa, Errore: errore, Tentativi: c.Tentativi,
		EsitoRimozione: testo(motivoRimozione), ConservazioneID: c.ConservazioneID, StatoAtteso: c.Stato}
	if stato == "da_rilasciare" && (c.Stato == "da_rilasciare" || c.Stato == "in_rimozione") {
		p.Tentativi = c.Tentativi + 1
		p.ProssimoTentativoIl = s.traDopo(attesaDopo(p.Tentativi))
	}
	if stato == "in_copia" || stato == "copiata" {
		p.Causa = pgtype.Text{}
	}
	if stato != "da_rilasciare" && stato != "rilasciata" {
		p.EsitoRimozione = pgtype.Text{}
	}
	n, err := q.AggiornaConservazione(ctx, p)
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("copia di conservazione: lo stato e' cambiato durante il passo")
	}
	azione := ""
	switch stato {
	case "rilasciata", "trattenuta":
		// trattenuta (base B di U2-Q01): la copia esce dal ciclo e il file resta; l'evento e' quello del rilascio, con lo
		// stato vero nell'esito (ck_fre_azione non ha un'azione propria)
		azione = "conservazione_rilasciata"
	case "errore_copia":
		azione = "conservazione_errore"
	default:
		return nil
	}
	esito := map[string]any{"stato": stato, "conservazione_id": c.ConservazioneID}
	if causa.Valid {
		esito["causa"] = causa.String
	}
	if motivoRimozione != "" {
		esito["rimozione"] = motivoRimozione
	}
	_, err = scriviEvento(ctx, q, c.ThreadID, evento{o: Occorrenza{db.ListOccorrenzeSorgenteRow{AllegatoID: c.AllegatoID}},
		azione: azione, esito: esito, origine: OrigineServer})
	return err
}

// togliFile: os.Remove, con un file assente che vale come fatto.
func togliFile(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// fileOccupato: l'errore di Windows di un file aperto da un altro programma (ERROR_SHARING_VIOLATION,
// ERROR_LOCK_VIOLATION).
func fileOccupato(err error) bool {
	var e syscall.Errno
	if errors.As(err, &e) {
		return e == 32 || e == 33
	}
	return false
}
