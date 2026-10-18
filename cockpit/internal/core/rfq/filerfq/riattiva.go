package filerfq

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// Riattiva e' S4, «Riattiva» (EM §10.6; D23:192, :204): il ritorno esplicito da «Altri file» ai file attivi. Non e' una
// conferma documentale ne' un'autorizzazione STEP: non crea documenti, non scrive step_strutturale_id ne'
// fonte_step_evento, non rimette associazioni e non tocca bozze.
//
// Fase A: i controlli e la copia verificata di ogni occorrenza (altrimenti 422 CONSERVAZIONE_MANCANTE). Fuori
// transazione: se lo staging non ha un file del contenuto, un parziale dal deposito con lo sha verificato scrivendo (e'
// solo un'ottimizzazione). Fase C: i ricontrolli, poi BloccaContenuto e, sotto il lucchetto, il controllo che decide
// (ContenutoGiaPresente sul file dello staging): se c'e' si usa, altrimenti si promuove il parziale (o si copia dal
// deposito dentro la transazione); lo stato attivo, l'evento, la copia da rilasciare, la strada d'ingresso
// dell'occorrenza, l'intento. Dopo il COMMIT, il rilascio della copia con le sue condizioni.
func (s *Servizio) Riattiva(ctx context.Context, thread, allegato, utente uuid.UUID, c CorpoGesto) (*EsitoGesto, error) {
	g, err := nuovoGesto(GestoRiattiva, thread, allegato, utente, c)
	if err != nil {
		return nil, err
	}
	// fase A
	var copie []copiaDaRiattivare
	var registrato *EsitoGesto
	err = s.inTx(ctx, func(q *db.Queries) error {
		pr, reg, err := s.prepara(ctx, q, g)
		if err != nil {
			return err
		}
		if reg != nil {
			registrato = reg
			return nil
		}
		copie, err = s.copieDaRiattivare(ctx, q, g, pr)
		return err
	})
	if err != nil {
		return nil, err
	}
	if registrato != nil {
		s.dopoIlGesto(ctx, thread, registrato.Occorrenze)
		return s.ricomponi(ctx, thread, registrato)
	}
	// fuori transazione: i parziali dal deposito dove lo staging non ha il contenuto
	for i := range copie {
		cp := &copie[i]
		if presenteNelloStaging(s.Staging, cp.destinazione, cp.occ.Sha()) {
			continue
		}
		parte, err := s.parzialeDalDeposito(cp)
		if err != nil {
			s.togliParziali(copie)
			return nil, err
		}
		cp.parte = parte
	}
	if s.Agganci != nil && s.Agganci.PrimaDellaFaseC != nil {
		s.Agganci.PrimaDellaFaseC(allegato)
	}
	// fase C
	var esito *EsitoGesto
	err = s.inTx(ctx, func(q *db.Queries) error {
		var err error
		esito, err = s.faseCRiattiva(ctx, q, g, copie)
		return err
	})
	s.togliParziali(copie) // i parziali non promossi (un rifiuto, o il contenuto gia' presente)
	if err != nil {
		return nil, err
	}
	s.dopoIlGesto(ctx, thread, esito.Occorrenze)
	return s.ricomponi(ctx, thread, esito)
}

// copiaDaRiattivare: un'occorrenza in Altri file del gesto, con la sua copia verificata e il posto nello staging.
type copiaDaRiattivare struct {
	occ          Occorrenza
	copia        db.FileRfqConservazione
	deposito     string // il file della copia nel deposito
	destinazione string // il posto del contenuto nello staging (staging.PercorsoContenuto)
	parte        string // il parziale scritto fuori transazione, se serve
	promossa     bool
}

// copieDaRiattivare: le occorrenze in Altri file del gesto (l'archivio e le sue voci in Altri file) con la copia
// copiata e il suo file presente con la dimensione attesa; altrimenti 422 CONSERVAZIONE_MANCANTE (EM §10.9, S4).
func (s *Servizio) copieDaRiattivare(ctx context.Context, q *db.Queries, g gesto, pr *preparato) ([]copiaDaRiattivare, error) {
	if s.Staging == "" {
		return nil, nonConfigurato("staging", "Lo staging non è configurato su questo server: niente è cambiato.")
	}
	var out []copiaDaRiattivare
	for _, o := range pr.tutte {
		if o.Stato != StatoAltriFile {
			continue
		}
		c, err := q.GetConservazioneCopiata(ctx, db.GetConservazioneCopiataParams{ThreadID: g.thread, AllegatoID: o.AllegatoID})
		if err != nil {
			return nil, conservazioneMancante(o)
		}
		if s.Deposito == nil || c.DepositoID != s.Deposito.ID {
			return nil, conservazioneMancante(o)
		}
		p, err := s.Deposito.PercorsoCopia(c.PercorsoRelativo)
		if err != nil || !c.Bytes.Valid || !s.Deposito.CopiaPresente(c.PercorsoRelativo, c.DepositoID, c.Bytes.Int64) {
			return nil, conservazioneMancante(o)
		}
		dest, err := staging.PercorsoContenuto(s.Staging, o.Sha(), o.NomeFile)
		if err != nil {
			return nil, conservazioneMancante(o)
		}
		out = append(out, copiaDaRiattivare{occ: o, copia: db.FileRfqConservazione{ConservazioneID: c.ConservazioneID,
			Sha256: c.Sha256, Bytes: c.Bytes, PercorsoRelativo: c.PercorsoRelativo, DepositoID: c.DepositoID},
			deposito: p, destinazione: dest})
	}
	return out, nil
}

func conservazioneMancante(o Occorrenza) *Rifiuto {
	return rifiuto(http.StatusUnprocessableEntity, "CONSERVAZIONE_MANCANTE",
		"Niente è cambiato: la copia di conservazione di «"+o.NomeFile+"» non c'è o non torna con lo sha256. È un'anomalia: segnalala.",
		extra("allegato_id", o.AllegatoID))
}

// parzialeDalDeposito copia il file del deposito in un parziale dello staging, verificando lo sha mentre scrive. Il
// parziale ha il nome del ciclo dei file (PrefissoParteCiclo), non quello dei tentativi dei worker.
func (s *Servizio) parzialeDalDeposito(cp *copiaDaRiattivare) (string, error) {
	parte := percorsoParteCiclo(s.Staging)
	causa, err := scriviParziale(cp.deposito, parte, cp.occ.Sha())
	if err != nil {
		_ = togliFile(parte)
		if causa == "verifica_sha" || causa == "sorgente_illeggibile" {
			return "", conservazioneMancante(cp.occ)
		}
		return "", err
	}
	return parte, nil
}

func (s *Servizio) togliParziali(copie []copiaDaRiattivare) {
	for _, cp := range copie {
		if cp.parte != "" && !cp.promossa {
			staging.RimuoviParte(cp.parte)
		}
	}
}

// faseCRiattiva: i ricontrolli sotto BloccaThread (con le proposte, in prepara), poi le righe delle copie FOR UPDATE,
// poi BloccaContenuto (sha in ordine crescente) e il controllo che decide sul file dello staging; lo stato attivo,
// l'evento, la copia in rilascio, la strada d'ingresso, l'intento. L'ordine e' quello di EM §10.6 (la RFQ, le righe,
// per ultimo il contenuto), con l'eccezione di chi adotta: l'allegato di cui si scrive path_staging si prende dopo il
// contenuto, nella transazione che lo verifica (EM §10.6, «chi adotta»), come fanno tutti gli scrittori di
// path_staging (il risultato dello stage, il riuso del gemello, il caricamento, la cache, il tempo 2). Prenderlo prima
// del contenuto incrocerebbe chi tiene il contenuto e aspetta l'allegato.
func (s *Servizio) faseCRiattiva(ctx context.Context, q *db.Queries, g gesto, copie []copiaDaRiattivare) (*EsitoGesto, error) {
	pr, reg, err := s.prepara(ctx, q, g)
	if err != nil {
		return nil, err
	}
	if reg != nil {
		return reg, nil
	}
	ora, err := s.copieDaRiattivare(ctx, q, g, pr)
	if err != nil {
		return nil, err
	}
	if len(ora) != len(copie) {
		return nil, perimetroCambiato(pr.per)
	}
	perID := map[uuid.UUID]*copiaDaRiattivare{}
	for i := range copie {
		perID[copie[i].occ.AllegatoID] = &copie[i]
	}
	for _, o := range ora {
		cp := perID[o.occ.AllegatoID]
		if cp == nil || cp.copia.ConservazioneID != o.copia.ConservazioneID {
			return nil, perimetroCambiato(pr.per)
		}
	}
	// le righe delle copie, FOR UPDATE, prima dei lucchetti per contenuto (in ordine di allegato, come le copie)
	righe := map[int64]db.FileRfqConservazione{}
	for _, cp := range copie {
		c, err := q.GetConservazionePerAggiornare(ctx, cp.copia.ConservazioneID)
		if err != nil {
			return nil, err
		}
		if c.Stato != "copiata" {
			return nil, conservazioneMancante(cp.occ)
		}
		righe[c.ConservazioneID] = c
	}
	sha := []string{}
	visto := map[string]bool{}
	for _, cp := range copie {
		if !visto[cp.occ.Sha()] {
			visto[cp.occ.Sha()] = true
			sha = append(sha, cp.occ.Sha())
		}
	}
	sort.Strings(sha)
	for _, h := range sha {
		if err := q.BloccaContenuto(ctx, h); err != nil {
			return nil, err
		}
	}
	ripristinati := false
	for i := range copie {
		cp := &copie[i]
		if !staging.ContenutoGiaPresente(cp.destinazione, cp.occ.Sha()) {
			if err := os.MkdirAll(filepath.Dir(cp.destinazione), 0o755); err != nil {
				return nil, err
			}
			if cp.parte == "" {
				// sparito proprio fra il passo 2 e il lucchetto: si copia dal deposito dentro la transazione (raro)
				parte, err := s.parzialeDalDeposito(cp)
				if err != nil {
					return nil, err
				}
				cp.parte = parte
			}
			if err := staging.Promuovi(cp.parte, cp.destinazione); err != nil {
				return nil, err
			}
			cp.promossa = true
			ripristinati = true
		} else {
			staging.ToccaContenuto(cp.destinazione)
		}
		// path_staging si rilegge sotto il lucchetto: la cache che ha tolto il contenuto dopo la fase A, o anche dopo la
		// lettura di prepara ma prima di BloccaContenuto, l'ha gia' azzerato (PM23 AF-11)
		attuale, err := q.GetAllegato(ctx, cp.occ.AllegatoID)
		if err != nil {
			return nil, err
		}
		if !attuale.PathStaging.Valid || !stessoPercorso(attuale.PathStaging.String, cp.destinazione) {
			if err := q.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{AllegatoID: cp.occ.AllegatoID,
				PathStaging: testo(cp.destinazione), Sha256: cp.occ.Sha256, Bytes: cp.occ.Bytes}); err != nil {
				return nil, err
			}
		}
	}
	ids := make([]uuid.UUID, 0, len(copie))
	for _, cp := range copie {
		ids = append(ids, cp.occ.AllegatoID)
	}
	if _, err := q.BloccaAllegatiPerFotografia(ctx, ids); err != nil {
		return nil, err
	}
	fresche, err := leggiOccorrenze(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	esito := &EsitoGesto{Esito: "riattivato", ChiaveIdempotenza: g.it.chiave, AllegatoID: g.allegato, Stato: StatoAttivo,
		Occorrenze: ids, ByteRipristinati: &ripristinati}
	for _, o := range fresche {
		ver, err := scriviStato(ctx, q, o, StatoAttivo, g.utente)
		if err != nil {
			return nil, err
		}
		if o.AllegatoID == g.allegato {
			esito.Versione = ver
		}
		if _, err := scriviEvento(ctx, q, g.thread, evento{o: o, azione: "riattiva", prima: StatoAltriFile, dopo: StatoAttivo,
			motivo: g.corpo.motivo(), fotografia: fotografia(o), origine: OriginePonte, autore: uid(g.utente),
			chiave: uid(g.it.chiave)}); err != nil {
			return nil, err
		}
		c := righe[perID[o.AllegatoID].copia.ConservazioneID]
		if _, err := q.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "da_rilasciare", Causa: testo("riattivato"),
			Tentativi: c.Tentativi, ConservazioneID: c.ConservazioneID, StatoAtteso: c.Stato}); err != nil {
			return nil, err
		}
	}
	// la strada d'ingresso dell'occorrenza: il file rientra come appena arrivato, senza un allegato nuovo (G-5: chiamata
	// dopo lo stato attivo)
	if s.Pipeline != nil {
		for _, id := range ids {
			if err := s.Pipeline.DopoCaricamento(ctx, q, id); err != nil {
				return nil, err
			}
		}
	}
	esito.Analisi = "in_analisi"
	if lav, err := q.ListLavoriPendentiSorgente(ctx, g.thread); err == nil {
		pend := map[string]bool{}
		for _, l := range lav {
			if l.AllegatoID == g.allegato {
				pend[l.Tipo] = true
			}
		}
		if a, err := q.GetAllegato(ctx, g.allegato); err == nil {
			esito.Analisi = statoAnalisi(a.Stato, a.Errore.String, pend).Stato
		}
	}
	esito.Conservazione = &StatoConservazione{AllegatoID: g.allegato, Stato: "da_rilasciare"}
	esito.Testo = "File riattivato: il motore lo rilegge e può proporlo di nuovo. Non conferma nessun documento e non autorizza nessuno STEP."
	if err := registraIntento(ctx, q, g.thread, g.it, esito); err != nil {
		return nil, err
	}
	return esito, nil
}
