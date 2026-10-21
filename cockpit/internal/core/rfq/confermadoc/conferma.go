// Package confermadoc: la conferma di una proposta in documento del Fascicolo, resa funzione del core (direttiva 35,
// blocco C-3, INC-G9; direttiva 39 §2: il passo 2 di S8). Fino a qui stava nel web (transport/web/allegati.go, conferma
// e confermaProposta) e leggeva la richiesta HTTP; adesso la chiamano la rotta legacy «Conferma il documento», il lotto
// del Fascicolo, «Documentazione generale» e S8, con la stessa logica e gli stessi testi.
//
// Che cosa fa (invariato rispetto al web, salvo la guardia di INC-G9): sulla transazione del chiamante, che ha gia'
// preso la RFQ (anche FOR KEY SHARE) e bloccato la proposta, crea il documento con la sua provenienza, chiude la
// proposta, scrive per uno STEP del finito la riga conferma_documento della storia della fonte, applica la scelta della
// revisione (aggiungi / sostituisce) e accoda la copia sul NAS. Lo stesso file gia' confermato nella RFQ diventa una
// provenienza in piu' (duplicato).
//
// La guardia di INC-G9 [T-39]: se l'allegato ha una associazione registrata attiva (file_rfq_associazione, servizio
// unico) il componente del documento deve essere quello della AR; un componente diverso, o nessun componente, e' un
// rifiuto. La strada «Sostituisci esplicito» del servizio unico non si apre qui: Sostituisci vuole una conferma della
// BOM formato 3 nella stessa transazione (D-AR01 A), e una AR esiste solo nelle RFQ formato 3, dove i gesti atomici
// non cambiano le AR (D-AR03 D [U]). Lo stesso controllo vale per il duplicato: il documento gia' presente deve stare
// sul componente della AR.
package confermadoc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// Rifiuto e' il «no» per l'operatore: lo stesso tipo del Fascicolo, cosi' le pagine legacy lo riconoscono con un solo
// errors.As e lo dicono con le loro parole.
type Rifiuto = fascicolo.Rifiuto

func rifiuto(s string) error { return Rifiuto(s) }

// I testi del file non attivo (EM §10.7): gli stessi delle guardie del web (sorgente_guardie.go).
const (
	TestoFileInAltriFile = "il file è in Altri file nella Distinta nuova (riattivalo prima)"
	TestoFileScartato    = "il file è stato scartato nella Distinta nuova: per riaverlo, ricaricalo"
)

// Gli esiti della copia sul NAS (U2 §2.2: copia_nas della risposta di S8).
const (
	CopiaInCoda         = "in_coda"
	CopiaScritturaSpent = "scrittura_spenta"
	CopiaNonPrevista    = "non_prevista"
)

// Scelta e' la risposta alla domanda che si fa quando un file entra in un componente che ha gia' un documento corrente
// dello stesso tipo (decisione del 24/09/2026 su A4.10): Aggiungi (Data vero, Sostituisce zero) oppure sostituisce un
// predecessore preciso. Senza risposta (Data falso) non si procede se la domanda c'e'.
type Scelta struct {
	Data        bool
	Sostituisce uuid.UUID
	// Riferimento: se il predecessore era lo STEP strutturale (o autorizzato) del componente, il nuovo ne prende il posto;
	// RispostaRiferimento dice che la domanda ha avuto una risposta (B8.7).
	Riferimento, RispostaRiferimento bool
	// SostituisceAutorizzato: il predecessore e' lo STEP autorizzato del componente (lo calcola Conferma con
	// fascicolo.StepAutorizzatoDi).
	SostituisceAutorizzato bool
	// Motivo: perche' si sostituisce; una versione interna (Interno) sostituisce solo con un motivo.
	Motivo  string
	Interno bool
}

// Richiesta: la conferma di una proposta. Proposta, Allegato e Messaggio li ha gia' letti chi chiama, con la proposta
// bloccata (BloccaProposta) dopo la RFQ.
type Richiesta struct {
	Utente     uuid.UUID
	Proposta   db.DocumentoProposta
	Allegato   db.Allegato
	Messaggio  db.Messaggio
	Tipo       db.TipoDocumento
	Codice     string
	Rev        string
	Nota       string
	Componente uuid.NullUUID
	Correggi   bool
	Scelta     Scelta
	// Fonte: i campi della riga conferma_documento della storia della fonte che S8 aggiunge (la chiave del passo, il
	// gruppo dell'anteprima, il motivo); vuoti per le pagine legacy, che la scrivono come prima.
	Fonte Fonte
}

// Fonte: i campi in piu' della riga conferma_documento (S8 passo 2, direttiva 39 §2).
type Fonte struct {
	Chiave uuid.UUID
	Gruppo string
	Motivo string
}

// Esito: che cosa e' successo. Messaggio e' la frase delle pagine legacy, invariata.
type Esito struct {
	Messaggio string
	Documento uuid.UUID
	// Duplicato: lo stesso file era gia' un documento della RFQ: solo una provenienza in piu'.
	Duplicato bool
	// FonteScritta: la riga conferma_documento della storia della fonte e' stata scritta.
	FonteScritta bool
	// Copia: CopiaInCoda, CopiaScritturaSpent, oppure CopiaNonPrevista (duplicato).
	Copia string
}

// Conferma crea il documento (commento di testa). Non crea componenti e non ne cerca uno per codice (B8.3): il documento
// si aggancia solo a r.Componente, se c'e', e ne prende il codice lettera per lettera (A1.4, A2.2). Un codice diverso da
// quello del componente passa solo con Correggi. Se il componente ha gia' un documento corrente dello stesso tipo serve
// la scelta: aggiungi, o sostituisce quale.
func Conferma(ctx context.Context, q *db.Queries, r Richiesta) (Esito, error) {
	p, a, m, tipo, codice, rev, comp := r.Proposta, r.Allegato, r.Messaggio, r.Tipo, r.Codice, r.Rev, r.Componente
	scelta := r.Scelta
	if p.Stato != db.StatoPropostaAperta {
		return Esito{}, errors.New("proposta già decisa")
	}
	// V1 (EM §10.7): un file in Altri file o scartato nella Distinta nuova non diventa un documento dalle pagine legacy
	// (conferma della voce, del lotto, «Documentazione generale») ne' da S8
	if err := RifiutoFileNonAttivo(ctx, q, a.AllegatoID); err != nil {
		return Esito{}, err
	}
	// `da_determinare` non è una destinazione (checkpoint 3R §5): non ha una sottocartella sul NAS.
	if tipo == db.TipoDocumentoDaDeterminare {
		return Esito{}, errors.New("il tipo di questo file non è ancora stato determinato: scegli il tipo, oppure lascia che il worker-analisi lo legga")
	}
	// Il codice e la revisione passano dalla stessa guardia degli altri (i nodi, l'editor, il worker).
	if codice != "" && !classificazione.CodiceAmmissibile(codice) {
		return Esito{}, rifiuto(fmt.Sprintf("%s: il codice ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxCodice))
	}
	if rev != "" && !classificazione.RevAmmissibile(rev) {
		return Esito{}, rifiuto(fmt.Sprintf("%s: la revisione ha più di %d caratteri o caratteri non ammessi", rev, classificazione.MaxRev))
	}
	if !m.ThreadID.Valid {
		return Esito{}, errors.New("il messaggio non è agganciato a una RFQ")
	}
	if !a.Sha256.Valid || !a.PathStaging.Valid {
		return Esito{}, errors.New("file non ancora scaricato")
	}
	if _, err := os.Stat(a.PathStaging.String); err != nil {
		return Esito{}, errors.New("file non presente in staging: usa Riscarica")
	}
	t, err := q.GetThread(ctx, m.ThreadID.UUID)
	if err != nil {
		return Esito{}, err
	}
	if !t.CartellaRelativa.Valid {
		return Esito{}, errors.New("thread senza cartella")
	}
	cl, err := q.GetCliente(ctx, t.ClienteID)
	if err != nil {
		return Esito{}, err
	}
	layout, err := q.GetCartellaDocumento(ctx, tipo)
	if err != nil {
		return Esito{}, fmt.Errorf("layout per %s: %w", tipo, err)
	}
	perCodice := RegolaBool(cl.Regole, "cartella_per_codice", true)

	// INC-G9 (commento di testa): la AR attiva dell'allegato decide il componente
	ar, err := associazioni.Attiva(ctx, q, t.ThreadID, a.AllegatoID)
	if err != nil {
		return Esito{}, err
	}

	// stesso file già confermato nel thread → solo una provenienza in più; ma se quel documento è stato sostituito,
	// tornare ai suoi byte non è «già presente» (addendum A4.1, R2.6)
	if d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: t.ThreadID, Sha256: a.Sha256.String}); err == nil {
		if d.SostituitoDa.Valid {
			return Esito{}, IdenticoAUnaRevisioneSostituita(ctx, q, d)
		}
		if err := guardiaAR(ctx, q, ar, d.ComponenteID); err != nil {
			return Esito{}, err
		}
		// la provenienza e la proposta chiusa sono la risposta: se una delle due non si scrive, la frase mentirebbe
		if err := q.InsertProvenienza(ctx, db.InsertProvenienzaParams{DocumentoID: d.DocumentoID, AllegatoID: uuid.NullUUID{UUID: a.AllegatoID, Valid: true},
			MessaggioID: uuid.NullUUID{UUID: m.MessaggioID, Valid: true}, RicevutoIl: a.RicevutoIl}); err != nil {
			return Esito{}, err
		}
		if _, err := q.DecidiProposta(ctx, db.DecidiPropostaParams{PropostaID: p.PropostaID, Stato: db.StatoPropostaDuplicato, DecisoDa: uuid.NullUUID{UUID: r.Utente, Valid: true}}); err != nil {
			return Esito{}, err
		}
		return Esito{Messaggio: "File già presente nel fascicolo (" + d.PathRelativo + "): registrata la nuova provenienza.", Documento: d.DocumentoID,
			Duplicato: true, Copia: CopiaNonPrevista}, nil
	}
	if err := guardiaAR(ctx, q, ar, comp); err != nil {
		return Esito{}, err
	}

	// Il componente scelto (B8.2: la struttura la decide chi assegna, con un gesto suo).
	codiceTxt := ptxt(codice)
	assegnato := ""
	// V1 (EM §2.3, §2.4, §9.10; PM23 R-24): il prodotto, quando il documento e' uno STEP del finito: la conferma scrive la
	// sua riga della storia della fonte
	var fonteDi uuid.UUID
	if comp.Valid {
		c, err := q.GetComponente(ctx, comp.UUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Esito{}, errors.New("il componente scelto non esiste")
		}
		if err != nil {
			return Esito{}, err
		}
		// il messaggio per l'operatore; chi impedisce l'aggancio e' la FK (thread, componente, codice)
		if c.ThreadID != t.ThreadID {
			return Esito{}, fmt.Errorf("il componente %s non è di questa RFQ", c.Codice)
		}
		if c.Tipo == db.TipoComponenteFinito && tipo == db.TipoDocumentoCad3d && FileStep(a.Estensione.String, a.NomeFile) {
			fonteDi = c.ComponenteID
		}
		if codice, err = CodiceDaComponente(codice, c, r.Correggi); err != nil {
			return Esito{}, err
		}
		codiceTxt = pgtype.Text{String: codice, Valid: true} // la stringa del componente, identica
		assegnato = c.Codice
		// Con la BOM congelata un file non si conferma gia' assegnato (D26).
		if n, bloccata, err := fascicolo.WorkingBloccata(ctx, q, t.ThreadID); err != nil {
			return Esito{}, err
		} else if bloccata {
			return Esito{}, rifiuto(fmt.Sprintf("la BOM è congelata nella V%d: il file entra senza componente, e lo si assegna aprendo una revisione", n))
		}
		correnti, err := CorrentiDelloStessoTipo(ctx, q, c.ComponenteID, tipo, uuid.Nil)
		if err != nil {
			return Esito{}, err
		}
		scelta.Interno = a.Origine == db.OrigineAllegatoManuale
		if scelta.Sostituisce != uuid.Nil {
			if scelta.SostituisceAutorizzato, err = fascicolo.StepAutorizzatoDi(ctx, q, t.ThreadID, c, scelta.Sostituisce); err != nil {
				return Esito{}, err
			}
		}
		if scelta, err = VerificaScelta(a.NomeFile, c, tipo, correnti, scelta); err != nil {
			return Esito{}, err
		}
	}

	// Un file tecnico senza codice non diventa documento (addendum A2.2).
	if documenti.Tecnico(tipo) && codice == "" {
		return Esito{}, errors.New("un CAD 3D, un disegno 2D o uno sviluppo DXF entra nel fascicolo solo con il codice del pezzo: scrivi il codice, il file resta fra le proposte")
	}
	// Il posto sul NAS (A4.1, D21, D22).
	cartella, err := documenti.CartellaDocumento(documenti.LayoutDocumento{Sottocartella: layout.Sottocartella, PerCodice: layout.PerCodice}, perCodice, codice)
	if err != nil {
		return Esito{}, fmt.Errorf("%s: %w", tipo, err)
	}
	pathRel, err := documenti.ScegliPercorso(ctx, q, t.ThreadID, cartella,
		documenti.NomeSulNas(tipo, codice, rev, a.Estensione.String, a.NomeFile))
	if err != nil {
		return Esito{}, err
	}
	// nome_file e' il nome ORIGINALE, com'e' arrivato (R1.6): il nome sul NAS vive solo in path_relativo.
	d, err := q.InsertDocumento(ctx, db.InsertDocumentoParams{
		ThreadID: t.ThreadID, ComponenteID: comp, Tipo: tipo, Codice: codiceTxt, Rev: ptxt(rev), NomeFile: a.NomeFile,
		Estensione: strings.ToLower(a.Estensione.String), Sha256: a.Sha256.String, Bytes: a.Bytes, PathRelativo: pathRel, ConfermatoDa: r.Utente, Nota: ptxt(r.Nota),
	})
	if err != nil {
		return Esito{}, fmt.Errorf("documento: %w", err)
	}
	if err := q.InsertProvenienza(ctx, db.InsertProvenienzaParams{DocumentoID: d.DocumentoID, AllegatoID: uuid.NullUUID{UUID: a.AllegatoID, Valid: true},
		MessaggioID: uuid.NullUUID{UUID: m.MessaggioID, Valid: true}, RicevutoIl: a.RicevutoIl}); err != nil {
		return Esito{}, err
	}
	if _, err := q.DecidiProposta(ctx, db.DecidiPropostaParams{PropostaID: p.PropostaID, Stato: db.StatoPropostaConfermata, DecisoDa: uuid.NullUUID{UUID: r.Utente, Valid: true}}); err != nil {
		return Esito{}, err
	}
	out := Esito{Documento: d.DocumentoID}
	if fonteDi != uuid.Nil {
		if err := ScriviConfermaDocumento(ctx, q, t.ThreadID, fonteDi, d.DocumentoID, a.AllegatoID, a.Sha256.String, r.Utente, r.Fonte); err != nil {
			return Esito{}, err
		}
		out.FonteScritta = true
	}
	componente := " Senza componente: codice e percorso restano questi finché non lo si assegna."
	if assegnato != "" {
		revisione, err := ApplicaScelta(ctx, q, t.ThreadID, d.DocumentoID, scelta, r.Utente)
		if err != nil {
			return Esito{}, err
		}
		// con la sostituzione dello STEP autorizzato e la risposta «sì» la riga «autorizzazione» del file nuovo la scrive
		// DichiaraStrutturale, che Sostituisci chiama (EM §2.4; terzo giro su O-01, OT-01)
		componente = " Assegnato al componente " + assegnato + "." + revisione
	}
	if _, err := coda.AccodaCopia(ctx, q, d.DocumentoID); err != nil {
		// Con `nas_scrittura` spenta il documento si conferma lo stesso e resta `in_coda`: la decisione dell'operatore è
		// registrata, è la SCRITTURA sul NAS che aspetta (SH1).
		if errors.Is(err, coda.ErrCapacitaSpenta) {
			out.Messaggio = "Confermato: " + pathRel + " — copia sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta." + componente
			out.Copia = CopiaScritturaSpent
			return out, nil
		}
		return Esito{}, err
	}
	out.Messaggio = "Confermato: " + pathRel + " (copia sul NAS in coda)." + componente
	out.Copia = CopiaInCoda
	return out, nil
}

// ScriviConfermaDocumento: la riga conferma_documento della storia della fonte del prodotto (ruolo prodotto, con il
// documento). Con f vuota e' la riga delle pagine legacy (fascicolo.ScriviFonteProdotto, come prima); con la chiave del
// passo e' la riga di S8, con il gruppo dell'anteprima e il motivo.
func ScriviConfermaDocumento(ctx context.Context, q *db.Queries, thread, prodotto, documento, allegato uuid.UUID, sha string, utente uuid.UUID, f Fonte) error {
	if f.Chiave == uuid.Nil {
		_, err := fascicolo.ScriviFonteProdotto(ctx, q, fascicolo.EventoFonte{Thread: thread, Prodotto: prodotto, Documento: documento,
			Allegato: allegato, Sha256: sha, Azione: fascicolo.FonteConfermaDocumento, Autore: utente})
		return err
	}
	_, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: thread, ProdottoID: prodotto, Ruolo: "prodotto",
		Azione: fascicolo.FonteConfermaDocumento, AllegatoID: allegato, Sha256: strings.TrimSpace(sha), Gruppo: ptxt(f.Gruppo),
		DocumentoID: uuid.NullUUID{UUID: documento, Valid: true}, Motivo: ptxt(tronca(f.Motivo, 1000)),
		ChiaveIdempotenza: uuid.NullUUID{UUID: f.Chiave, Valid: true}, AutoreID: utente})
	return err
}

// guardiaAR: INC-G9 (commento di testa). ar nil = nessuna AR attiva: nessun vincolo.
func guardiaAR(ctx context.Context, q *db.Queries, ar *associazioni.Associazione, comp uuid.NullUUID) error {
	if ar == nil {
		return nil
	}
	if comp.Valid && comp.UUID == ar.Componente {
		return nil
	}
	nome := ar.Componente.String()
	if c, err := q.GetComponente(ctx, ar.Componente); err == nil {
		nome = c.Codice
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if !comp.Valid {
		return rifiuto(fmt.Sprintf("il file ha un'associazione registrata con %s: il documento entra sotto quel componente, non senza componente", nome))
	}
	return rifiuto(fmt.Sprintf("il file ha un'associazione registrata con %s: per un altro componente si cambia la preassociazione nella bozza e si conferma la BOM", nome))
}

// RifiutoFileNonAttivo: il rifiuto di un gesto documentale su un file non attivo (in Altri file o scartato nella
// Distinta nuova), oppure nil. Nessuna riga dell'occorrenza = attivo.
func RifiutoFileNonAttivo(ctx context.Context, q *db.Queries, allegato uuid.UUID) error {
	st, err := q.StatoOccorrenzaAllegato(ctx, allegato)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	switch st.Stato {
	case filerfq.StatoAltriFile:
		return rifiuto(TestoFileInAltriFile)
	case filerfq.StatoScartato:
		return rifiuto(TestoFileScartato)
	}
	return nil
}

// IdenticoAUnaRevisioneSostituita e' il rifiuto di A4.1 (R2.6): il file e' identico a un documento che e' stato
// sostituito. Tornare ai suoi byte non puo' essere una riga nuova (UNIQUE (thread, sha256)), e dire «gia' presente»
// farebbe credere che il fascicolo sia a posto mentre resta sulla revisione piu' nuova.
func IdenticoAUnaRevisioneSostituita(ctx context.Context, q *db.Queries, d db.Documento) error {
	storia, err := q.ListStoriaDocumento(ctx, d.DocumentoID)
	if err != nil {
		return err
	}
	var dopo []db.VDocumentoStoria
	trovato := false
	for _, s := range storia {
		if trovato {
			dopo = append(dopo, s)
		}
		if s.DocumentoID == d.DocumentoID {
			trovato = true
		}
	}
	switch len(dopo) {
	case 0:
		return rifiuto(fmt.Sprintf("il file è identico a %s, che risulta sostituito: ricarica la pagina", d.NomeFile))
	case 1:
		return rifiuto(fmt.Sprintf("il file è identico a %s, che %s ha sostituito: per tornare a %s si annulla quella sostituzione",
			d.NomeFile, dopo[0].NomeFile, d.NomeFile))
	}
	return rifiuto(fmt.Sprintf("il file è identico a %s, sostituito da %s e poi da %s: tornare a una revisione più vecchia della precedente non è ancora gestito",
		d.NomeFile, dopo[0].NomeFile, dopo[len(dopo)-1].NomeFile))
}

// stessoCodice confronta due codici come li confronta l'identita' del componente, upper(codice).
func stessoCodice(a, b string) bool {
	return strings.ToUpper(strings.TrimSpace(a)) == strings.ToUpper(strings.TrimSpace(b))
}

// CodiceDaComponente e' il codice che un documento o una proposta prende quando si aggancia a c. attuale e' quello che
// hanno adesso ("" = nessuno). Lo stesso codice con altre maiuscole non e' una correzione (A1.4, regola 8); un codice
// DIVERSO lo diventa solo con il gesto esplicito «assegna e correggi il codice» (regola 1).
func CodiceDaComponente(attuale string, c db.Componente, correggi bool) (string, error) {
	if strings.TrimSpace(attuale) != "" && !stessoCodice(attuale, c.Codice) && !correggi {
		return "", rifiuto(fmt.Sprintf("codice diverso: %s contro %s. Per assegnarlo comunque va corretto il codice del file (correggi_codice)",
			strings.TrimSpace(attuale), c.Codice))
	}
	return c.Codice, nil
}

// CorrentiDelloStessoTipo sono i documenti correnti del componente con quel tipo, escluso uno. ListDocumentiComponente
// li blocca: due assegnazioni allo stesso pezzo si mettono in fila, e la seconda vede il file della prima.
func CorrentiDelloStessoTipo(ctx context.Context, q *db.Queries, comp uuid.UUID, tipo db.TipoDocumento, escluso uuid.UUID) ([]db.Documento, error) {
	tutti, err := q.ListDocumentiComponente(ctx, uuid.NullUUID{UUID: comp, Valid: true})
	if err != nil {
		return nil, err
	}
	var out []db.Documento
	for _, d := range tutti {
		if d.Tipo == tipo && !d.SostituitoDa.Valid && d.DocumentoID != escluso {
			out = append(out, d)
		}
	}
	return out, nil
}

// VerificaScelta applica la regola: con almeno un corrente dello stesso tipo serve la risposta, e un «sostituisce» deve
// nominare uno di quei correnti. nome e' il file che entra. Un «aggiungi» dato dove non c'era niente da chiedere non dice
// niente, e si lascia cadere.
func VerificaScelta(nome string, c db.Componente, tipo db.TipoDocumento, correnti []db.Documento, s Scelta) (Scelta, error) {
	if len(correnti) == 0 && s.Sostituisce == uuid.Nil {
		return Scelta{}, nil
	}
	if len(correnti) > 0 && !s.Data {
		nomi := make([]string, len(correnti))
		for i, d := range correnti {
			nomi[i] = d.NomeFile + " (" + d.DocumentoID.String() + ")"
		}
		return s, rifiuto(fmt.Sprintf("%s: %s ha già %d document%s %s corrent%s (%s). Si sceglie: «aggiungi» (restano correnti tutti) oppure il documento che questo sostituisce",
			nome, c.Codice, len(correnti), plurale(len(correnti), "o", "i"), tipo, plurale(len(correnti), "e", "i"), strings.Join(nomi, ", ")))
	}
	if s.Sostituisce == uuid.Nil {
		return s, nil
	}
	trovato := false
	for _, d := range correnti {
		trovato = trovato || d.DocumentoID == s.Sostituisce
	}
	if !trovato {
		return s, rifiuto(fmt.Sprintf("%s: il documento da sostituire deve essere un %s corrente di %s", nome, tipo, c.Codice))
	}
	return s, VerificaSostituzione(nome, c, s)
}

// VerificaSostituzione sono le due domande in piu' di una sostituzione (B8.7): se il predecessore e' lo STEP strutturale
// del componente, si dice esplicitamente se il nuovo diventa il riferimento; se il file nuovo e' una versione interna,
// si dice perche'.
func VerificaSostituzione(nome string, c db.Componente, s Scelta) error {
	autorizzato := s.SostituisceAutorizzato || (c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == s.Sostituisce)
	if autorizzato && s.Sostituisce != uuid.Nil && !s.RispostaRiferimento {
		return rifiuto(fmt.Sprintf("%s sostituisce lo STEP autorizzato di %s: si dice se il nuovo diventa il riferimento, cioè lo STEP autorizzato (sì o no)", nome, c.Codice))
	}
	if s.Interno && s.Motivo == "" {
		return rifiuto(fmt.Sprintf("%s è una versione interna: sostituisce un documento solo con un motivo", nome))
	}
	return nil
}

// ApplicaScelta registra la risposta dopo che il file nuovo e' entrato nel componente: la sostituzione e' quella di
// fascicolo.Sostituisci, con la catena tenuta dal database (D32). utente e' chi decide: con «si'» autorizza il file
// nuovo (Smistamento F5b).
func ApplicaScelta(ctx context.Context, q *db.Queries, thread, nuovo uuid.UUID, s Scelta, utente uuid.UUID) (string, error) {
	if !s.Data {
		return "", nil
	}
	if s.Sostituisce == uuid.Nil {
		return " Si aggiunge: i documenti dello stesso tipo restano correnti.", nil
	}
	msg, err := fascicolo.Sostituisci(ctx, q, thread, s.Sostituisce, nuovo, s.Riferimento, utente)
	if err != nil {
		return "", err
	}
	if err := NotaSostituzione(ctx, q, s.Sostituisce, nuovo, s.Motivo); err != nil {
		return "", err
	}
	return " " + msg, nil
}

// NotaSostituzione scrive nella nota del documento nuovo che cosa ha sostituito e perche'. Senza motivo non scrive
// niente: la catena dice gia' chi ha sostituito chi.
func NotaSostituzione(ctx context.Context, q *db.Queries, vecchio, nuovo uuid.UUID, motivo string) error {
	if motivo == "" {
		return nil
	}
	v, err := q.GetDocumento(ctx, vecchio)
	if err != nil {
		return err
	}
	n, err := q.GetDocumento(ctx, nuovo)
	if err != nil {
		return err
	}
	nota := "sostituisce " + v.NomeFile + ": " + motivo
	if n.Nota.Valid && strings.TrimSpace(n.Nota.String) != "" {
		nota = n.Nota.String + " · " + nota
	}
	return q.SetNotaDocumento(ctx, db.SetNotaDocumentoParams{DocumentoID: nuovo, Nota: pgtype.Text{String: nota, Valid: true}})
}

// FileStep dice se un file e' uno STEP (estensione stp o step; senza estensione registrata, il nome).
func FileStep(estensione, nome string) bool {
	e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(estensione), "."))
	if e == "" {
		if i := strings.LastIndex(nome, "."); i >= 0 {
			e = strings.ToLower(nome[i+1:])
		}
	}
	return e == "stp" || e == "step"
}

// RegolaBool legge una regola booleana del cliente (cliente.regole), con il valore predefinito.
func RegolaBool(regole json.RawMessage, chiave string, def bool) bool {
	var m map[string]any
	if json.Unmarshal(regole, &m) != nil {
		return def
	}
	if v, ok := m[chiave].(bool); ok {
		return v
	}
	return def
}

func ptxt(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func tronca(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func plurale(n int, uno, molti string) string {
	if n == 1 {
		return uno
	}
	return molti
}
