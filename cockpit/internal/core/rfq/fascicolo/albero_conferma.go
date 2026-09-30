package fascicolo

// «Conferma l'albero» (giro 4, fase 4.4a.1b; domande 27 = A, 28 = A, 29b = A, 30 seconda risposta, 6a, 6b; studio
// docs/specs/studio_albero_distinta_29-09.md § 2.8 e § 2.9, con la verifica in coda). E' il solo gesto della Distinta
// che fa nascere componenti e relazioni dall'albero proposto: il confine umano del modello «albero proposto, poi
// confermato» (27 = A, «nessuna identita' senza una persona»).
//
// Due meta', come l'autorizzazione di F5b. Il riepilogo (albero_riepilogo.go) e' una lettura: dice che cosa la
// conferma farebbe, con la sua firma. La conferma, sotto il lucchetto della RFQ e con la BOM libera (prepara, D26),
// ricalcola lo stesso riepilogo sulla stessa bozza e si rifiuta senza scrivere niente se la firma non e' quella che la
// persona ha visto (l'albero, la working o la storia dei componenti sono cambiati) o se il riepilogo non e'
// confermabile (una proposta commerciale senza ✓ o ✗, un codice quasi uguale senza «diverso», una quantita' discorde
// senza scelta, un nodo senza codice, una bozza disegnata su un albero vecchio, un blocco sulla struttura). Poi scrive,
// in UNA transazione (quella di chi chiama), quello che il riepilogo dice, con gli stessi conti (pianoConferma):
//   - i pezzi che nascono (codice, tipo, revisione e descrizione dai file o dalla bozza) e quelli che si ritrovano per
//     lo stesso codice, solo quelli che il riepilogo elenca (P13; un archiviato si ripristina);
//   - i tipi: quello proposto dall'albero o scelto nella bozza, con chi ha confermato (confermato_da). Il particolare
//     commerciale nasce solo da un gesto di una persona, in due modi: il ✓ della proposta commerciale (o la tendina su
//     un nodo con la proposta, che e' lo stesso gesto), che passa da tipoConfermatoNellAlbero e lascia nel segno la
//     risposta; oppure la tendina su un nodo senza proposta, che e' il tipo scelto da una persona come nel modulo della
//     scheda (arriva gia' nel tipo del riepilogo, «scelto nella bozza» fra i tipi, senza un segno commerciale: non c'e'
//     una proposta a cui rispondere). Il tipo di un componente che c'e' cambia con cambiaTipo, le regole della scheda
//     (il commerciale sospende le autorizzazioni), e il riepilogo ne dice prima l'effetto (effettiDeiTipi);
//   - la revisione nuova di un componente che c'e' («7121003: rev A → rev B»): una revisione aggiorna lo stesso pezzo,
//     non ne crea uno nuovo (risposta 28);
//   - i legami con le loro quantita': prima si tolgono (la cascata, 29b = A), poi le quantita', poi si aggiungono;
//     il grafo finale e' senza cicli (il riepilogo), e collega li ricontrolla uno per uno contro la working intera;
//   - le rimozioni proposte dallo STEP: accettate se il legame va via, chiuse («tenuto») se resta;
//   - i componenti che restano senza padri: archiviati se hanno documenti o storia, eliminati se no (29b = A; per
//     questo gesto cambia D29, «i figli diventano radici da sistemare», e il riepilogo lo dice pezzo per pezzo);
//   - le righe degli STEP che l'albero copre, decise da una persona con il segno «confermato nell'albero»: dei nodi che
//     restano, le righe ancora da decidere (le aperte e gli agganci per codice di prima dello Smistamento, che il
//     riepilogo elenca fra i ritrovati: U5, P13); dei nodi che vanno via, tutte.
//
// Il segno (studio § 2.9, strada A) sta nelle colonne e negli stati che ci sono, senza migrazione: la riga diventa
// una decisione di una persona come quando la si accetta a mano (confermata se ha fatto nascere il componente,
// duplicato se lo ritrova, scartata con la nota se la conferma la toglie; deciso_da e deciso_il) e porta in
// evidenza.albero chi, quando, la firma del riepilogo e il ✓ o il ✗ della proposta commerciale; la riga di un arco
// porta anche i due componenti del legame (la radice del prodotto non e' una riga decisa). Una riga decisa da una
// persona le letture non la riscrivono piu' (UpsertComponenteProposta, UpsertRelazioneProposta: solo le aperte e
// quelle chiuse da un automatismo), quindi il segno resta; «Riapri il nodo» lo porta nella storia e, per un nodo che la
// conferma ha tolto, riapre con lui le righe degli archi che la stessa conferma aveva chiuso (toltoNellAlbero), cosi'
// il nodo torna nell'albero proposto (studio § 2.5). Lo leggono:
//   - F7 (Provenienza e ArchiDecisiNellAutorita, riapertura.go): una riga con il segno conta come una decisione presa
//     nell'autorita' di un file autorizzato, e il pezzo e l'arco non sono «da rivedere»;
//   - l'indice di F8, con le stesse etichette (derivati.daRivedere e arcoDeciso): i pezzi dell'albero confermato
//     sono «decisi», e i loro file si preselezionano come dopo un'accettazione nello STEP autorizzato;
//   - il gate (GateStrutturale) conta come decisa una riga con chi l'ha decisa: le righe nell'autorita' di uno STEP
//     autorizzato che la conferma copre non lo fermano piu', e la guida decisa non e' piu' un avviso.
// Senza il segno la strada era una sola: accettare i nodi come accettaNodo (origine step, fuori dall'autorita') e
// lasciare che F7 li segnasse «da rivedere» e che F8 non preselezionasse niente; oppure farli nascere come carte a
// mano, perdendo il legame con i file e lasciando aperte le righe degli STEP (il Bug 2).
//
// Perche' una funzione nuova e non ApplicaStrutturaVoluta allargata: il contratto di quella e' l'editor del Fascicolo
// (una radice, solo l'autorita', gli archi e le carte che l'editor ha mostrato, P9 e la prova 97: nessun nodo di guida
// entra dall'editor). La conferma dell'albero ha un altro confine, il riepilogo firmato, che dice gia' tutto quello che
// si scrive (una foresta, i nodi di guida, i tipi, i codici, le revisioni, la cascata): allargare l'editor avrebbe
// tolto la sua guardia per dargli quella dell'albero. Le due condividono l'interno — prepara, collega e scollega,
// cambiaTipo, ArchiviaComponente e RimuoviComponente, la chiusura delle rimozioni, dopoLaDecisione — cosi' le regole
// della working sono una sola.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// ChiaveSegnoAlbero e' la chiave di evidenza (componente_proposta, relazione_proposta) dove la conferma dell'albero
// lascia il suo segno.
const ChiaveSegnoAlbero = "albero"

// NotaToltoNellAlbero e' la nota delle righe che la conferma dell'albero chiude perche' la bozza le toglie.
const NotaToltoNellAlbero = "tolto nell'albero confermato"

// SegnoAlbero e' il segno «confermato nell'albero» su una riga decisa dalla conferma.
type SegnoAlbero struct {
	Da    uuid.UUID `json:"da"`
	Il    string    `json:"il"`
	Firma string    `json:"firma"`
	// Padre e Figlio: i componenti del legame, sulla riga di un arco che la conferma tiene.
	Padre  uuid.NullUUID `json:"padre"`
	Figlio uuid.NullUUID `json:"figlio"`
	// Commerciale: la risposta di una persona alla proposta commerciale del nodo.
	Commerciale *SegnoCommerciale `json:"commerciale,omitempty"`
}

// SegnoCommerciale e' il ✓ o il ✗ sulla proposta commerciale, con il motivo della proposta: cosi' il censimento conta
// anche i «mancati», le proposte che l'ingegnere ha scartato (risposta 30).
type SegnoCommerciale struct {
	Risposta string `json:"risposta"` // RispostaSi, RispostaNo
	Tendina  bool   `json:"tendina,omitempty"`
	Motivo   string `json:"motivo"`
}

// ConfermatoNellAlbero dice se la riga di un nodo e' una decisione di una persona presa con la conferma dell'albero.
func ConfermatoNellAlbero(p db.ComponenteProposta) bool {
	_, deciso := DecisoDaUnaPersona(p)
	return deciso && haChiave(p.Evidenza, ChiaveSegnoAlbero)
}

// ArcoConfermatoNellAlbero dice se la riga di un arco e' un legame tenuto dalla conferma dell'albero, e quale: i due
// componenti che il segno dice.
func ArcoConfermatoNellAlbero(r db.RelazioneProposta) (CoppiaDiComponenti, bool) {
	if !r.DecisoDa.Valid || (r.Stato != db.StatoPropostaConfermata && r.Stato != db.StatoPropostaDuplicato) {
		return CoppiaDiComponenti{}, false
	}
	var e struct {
		Albero *SegnoAlbero `json:"albero"`
	}
	if json.Unmarshal(r.Evidenza, &e) != nil || e.Albero == nil || !e.Albero.Padre.Valid || !e.Albero.Figlio.Valid {
		return CoppiaDiComponenti{}, false
	}
	return CoppiaDiComponenti{e.Albero.Padre.UUID, e.Albero.Figlio.UUID}, true
}

// toltoNellAlbero dice se la riga di un nodo l'ha chiusa una conferma dell'albero perche' la bozza lo toglieva, e con
// quale firma: «Riapri il nodo» riapre con lei le righe degli archi che la stessa conferma aveva chiuso (RiapriNodo).
func toltoNellAlbero(p db.ComponenteProposta) (string, bool) {
	if p.Stato != db.StatoPropostaScartata || !p.DecisoDa.Valid || p.Nota.String != NotaToltoNellAlbero {
		return "", false
	}
	var e struct {
		Albero *SegnoAlbero `json:"albero"`
	}
	if json.Unmarshal(p.Evidenza, &e) != nil || e.Albero == nil || e.Albero.Firma == "" {
		return "", false
	}
	return e.Albero.Firma, true
}

// tipoConfermatoNellAlbero e' il tipo con cui nasce un pezzo dell'albero confermato: quello del riepilogo (proposto
// dall'albero, che per un pezzo che nasce non e' mai commerciale, o scelto con la tendina da una persona), o il
// particolare commerciale quando una persona ha confermato la proposta commerciale (il ✓, o la tendina). E' il solo
// posto, fuori da un confronto e dalle tendine dei tipi, dove il codice nomina il tipo commerciale: la guardia
// TestNessunAutomatismoScriveIlTipoCommerciale lo ammette per nome, e ammette una chiamata sola, in ConfermaAlbero con
// la risposta della bozza, perche' qui il tipo lo sceglie una persona (domanda 30, seconda risposta: «la
// responsabilita' e' di chi conferma») e la conferma registra chi (confermato_da, e il segno sulle righe). Senza la
// conferma di una persona non da' mai il commerciale.
func tipoConfermatoNellAlbero(proposto db.TipoComponente, confermatoDaUnaPersona bool) db.TipoComponente {
	if confermatoDaUnaPersona {
		return db.TipoComponenteCommerciale
	}
	return proposto
}

// CorpoConferma e' il corpo della POST della conferma: la firma del riepilogo che la persona ha visto e la bozza su cui
// e' stato calcolato.
type CorpoConferma struct {
	Firma string          `json:"firma"`
	Bozza json.RawMessage `json:"bozza"`
}

// LeggiConferma legge il corpo della conferma: la firma e la bozza (LeggiBozza, con il suo formato).
func LeggiConferma(raw []byte) (BozzaAlbero, string, error) {
	var c CorpoConferma
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil || dec.More() {
		return BozzaAlbero{}, "", Rifiuto("la conferma dell'albero non si legge: ricarica la pagina")
	}
	if strings.TrimSpace(c.Firma) == "" {
		return BozzaAlbero{}, "", Rifiuto("la conferma arriva senza la firma del riepilogo: rileggi il riepilogo e conferma di nuovo")
	}
	b, err := LeggiBozza(c.Bozza)
	return b, strings.TrimSpace(c.Firma), err
}

// esitoConferma conta quello che la conferma ha fatto, per la frase all'operatore.
type esitoConferma struct {
	nuovi, ritrovati, ripristinati, commerciali, tipi, revisioni, legami, quantita, tolti, righe, chiuse int
	archiviati, eliminati                                                                                []string
	parti                                                                                                []string
}

// ConfermaAlbero conferma l'albero proposto con la bozza della persona: vedi l'intestazione del file. firma e' la
// firma del riepilogo che la persona ha visto. Una sola transazione, quella di chi chiama: un rifiuto a meta' (un
// ciclo con un arco fuori dall'albero, un componente che la FK non lascia eliminare) annulla tutto.
func ConfermaAlbero(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, an coda.Analizzatore, b BozzaAlbero, firma string) (string, error) {
	if err := prepara(ctx, q, thread, "si conferma l'albero"); err != nil {
		return "", err
	}
	_, r, pc, err := leggiRiepilogo(ctx, q, thread, an, &b)
	if err != nil {
		return "", err
	}
	if firma == "" || firma != r.Firma {
		return "", Rifiuto("quello che la conferma farebbe è cambiato da quando hai letto il riepilogo (una decisione nel frattempo, " +
			"un file nuovo): rileggi il riepilogo e conferma di nuovo")
	}
	if !r.Confermabile {
		return "", Rifiuto("l'albero non si conferma ancora: " + strings.Join(r.Blocchi, "; "))
	}
	segno := SegnoAlbero{Da: utente, Il: time.Now().UTC().Format(time.RFC3339), Firma: r.Firma}
	var es esitoConferma
	compDi := map[*gruppo]uuid.UUID{}
	idNodo := func(chiave string) (uuid.UUID, error) {
		if n, ok := pc.a.Nodo(chiave); ok && n.Componente.Valid {
			return n.Componente.UUID, nil
		}
		return uuid.Nil, Rifiuto(chiave + ": il componente del legame non si trova: rileggi il riepilogo")
	}

	// 1. i codici scritti nella bozza, sulle righe dei nodi rinominati: come «Correggi il codice del nodo»
	// (CodiceDelNodo, origine operatore), perche' una rilettura non li tocchi piu'
	for _, g := range pc.gruppi {
		for _, m := range g.membri {
			if !m.rinominato || m.albero == nil {
				continue
			}
			for _, rr := range m.albero.Righe {
				if rr.Stato != db.StatoPropostaAperta {
					continue
				}
				rev := m.revScritta
				if rev == "" {
					rev = rr.Rev
				}
				if _, err := q.SetCodiceComponenteProposta(ctx, db.SetCodiceComponentePropostaParams{PropostaID: rr.Proposta,
					Codice: pgtype.Text{String: m.codice, Valid: true}, Rev: testo(rev)}); err != nil {
					return "", err
				}
			}
		}
	}

	// 2. i legami della working che vanno via (la cascata), e le rimozioni proposte dallo STEP: accettate se il legame
	// va via, chiuse se resta
	toltiOra := map[*ArcoAlbero]bool{}
	for _, l := range pc.tolti {
		p, err := idNodo(l.Padre)
		if err != nil {
			return "", err
		}
		f, err := idNodo(l.Figlio)
		if err != nil {
			return "", err
		}
		if _, err := scollega(ctx, q, thread, p, f); err != nil {
			return "", err
		}
		toltiOra[l] = true
		es.tolti++
	}
	for _, l := range pc.rimozioni {
		p, err := idNodo(l.Padre)
		if err != nil {
			return "", err
		}
		f, err := idNodo(l.Figlio)
		if err != nil {
			return "", err
		}
		if toltiOra[l] {
			if _, err := q.DecidiRimozione(ctx, db.DecidiRimozioneParams{ThreadID: thread, StepDocumentoID: l.Rimozione.Step, PadreID: p,
				FiglioID: f, Stato: db.StatoPropostaConfermata, DecisoDa: uid(utente)}); err != nil {
				return "", err
			}
			continue
		}
		if _, err := q.TieniArcoDellaRimozione(ctx, db.TieniArcoDellaRimozioneParams{ThreadID: thread, StepDocumentoID: l.Rimozione.Step,
			PadreID: p, FiglioID: f, DecisoDa: uid(utente)}); err != nil {
			return "", err
		}
	}

	// 3. i pezzi: il prodotto che non ha ancora il suo componente nasce come alla preparazione; un ritrovato archiviato
	// si ripristina; un pezzo nuovo nasce con il codice, il tipo, la revisione e la descrizione del riepilogo
	for _, g := range pc.gruppi {
		switch {
		case g.esiste:
			if g.comp.ArchiviatoIl != nil {
				if _, err := q.RipristinaComponente(ctx, g.comp.ComponenteID); err != nil {
					return "", err
				}
				es.ripristinati++
			}
			compDi[g] = g.comp.ComponenteID
		case g.prodotto:
			codice := g.codice
			if _, err := assicuraProdotti(ctx, q, thread, func(i db.IdentificativoThread) bool {
				return strings.EqualFold(strings.TrimSpace(i.Codice), codice)
			}); err != nil {
				return "", err
			}
			c, err := componenteNato(ctx, q, thread, codice)
			if err != nil {
				return "", err
			}
			compDi[g] = c.ComponenteID
			es.nuovi++
		default:
			rp := g.rappresentante()
			origine, descrizione := db.OrigineComponenteManuale, ""
			for _, m := range g.membri {
				if m.albero != nil {
					origine = db.OrigineComponenteStep
					if descrizione == "" {
						descrizione = m.albero.Descrizione
					}
				}
			}
			// il ✓ di una persona passa qui, e solo qui (la guardia TestNessunAutomatismoScriveIlTipoCommerciale vuole
			// questa chiamata, una sola, con la risposta della bozza come argomento)
			c, err := q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: thread, Codice: g.codice, Rev: testo(rp.rev),
				Descrizione: testo(taglia(descrizione, 200)), Qta: 1, Tipo: tipoConfermatoNellAlbero(g.tipo, g.commercialeConfermato()),
				Origine: origine, ConfermatoDa: utente})
			if err != nil {
				return "", err
			}
			compDi[g] = c.ComponenteID
			es.nuovi++
			if g.commercialeConfermato() {
				es.commerciali++
			}
		}
	}

	// 4. il tipo e la revisione dei componenti che c'erano: il tipo con le regole della scheda (cambiaTipo: un
	// commerciale sospende le autorizzazioni, un particolare che riceve dei figli diventa assieme prima degli archi).
	// L'effetto il riepilogo l'ha gia' detto, calcolato su come il componente e' adesso (effettiDeiTipi): uno spento
	// qui, dopo un riepilogo confermabile, resta un rifiuto che annulla tutto
	for _, g := range pc.gruppi {
		if !g.esiste || g.prodotto {
			continue
		}
		id := compDi[g]
		c, err := q.GetComponente(ctx, id)
		if err != nil {
			return "", err
		}
		if g.tipo != c.Tipo {
			e, err := EffettoCambioTipo(ctx, q, thread, id, g.tipo)
			if err != nil {
				return "", err
			}
			if e.Spento != "" {
				return "", Rifiuto(c.Codice + ": " + e.Spento)
			}
			parti, err := cambiaTipo(ctx, q, thread, e, utente)
			if err != nil {
				return "", err
			}
			es.tipi++
			es.parti = append(es.parti, parti...)
			if c, err = q.GetComponente(ctx, id); err != nil {
				return "", err
			}
		}
		if rev, ok := pc.revisioni[g.identita]; ok {
			if _, err := q.SetDatiComponente(ctx, db.SetDatiComponenteParams{ComponenteID: id, Tipo: c.Tipo, Rev: testo(rev),
				Descrizione: c.Descrizione, ConfermatoDa: utente}); err != nil {
				return "", err
			}
			es.revisioni++
		}
	}

	// 5. i legami: la quantita' di quelli che c'erano, poi quelli nuovi. Un legame che viene dalle righe di uno STEP
	// nasce con l'origine step (le righe restano legate a lui, con il segno); uno scritto nella bozza, a mano
	scritto := map[*legameFinale]bool{}
	for _, f := range pc.legami {
		p, fi := compDi[f.padre], compDi[f.figlio]
		ora, err := q.GetRelazione(ctx, db.GetRelazioneParams{PadreID: p, FiglioID: fi})
		if err == nil {
			if ora.Qta != f.qta {
				if _, err := q.SetQtaRelazione(ctx, db.SetQtaRelazioneParams{PadreID: p, FiglioID: fi, Qta: f.qta, ConfermatoDa: utente}); err != nil {
					return "", err
				}
				scritto[f] = true
				es.quantita++
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		origine := db.OrigineComponenteManuale
		if len(righeDelLegame(f)) > 0 {
			origine = db.OrigineComponenteStep
		}
		if _, _, err := collegaDa(ctx, q, thread, p, fi, utente, f.qta, origine); err != nil {
			return "", err
		}
		scritto[f] = true
		es.legami++
	}

	// 6. le righe degli STEP che l'albero copre, con il segno. I nodi: la prima riga di un pezzo nuovo lo ha fatto
	// nascere (confermata), le altre lo ritrovano (duplicato); le righe dei nodi che escono dall'albero si chiudono. Gli
	// archi: tenuti (confermata se la conferma ha scritto il legame, duplicato se c'era gia'), o chiusi.
	// Di un nodo che resta si decidono solo le righe ancora da decidere (RigaAlbero.daDecidere: le aperte e gli agganci
	// per codice di prima), e di un componente che c'e' solo quelle dei nodi che il riepilogo elenca fra i ritrovati
	// (P13: «solo i ritrovati elencati»; fase 4.4a.1b, dalla verifica). Una riga chiusa da un automatismo resta com'e'
	for _, g := range pc.gruppi {
		if g.prodotto {
			continue
		}
		stato := db.StatoPropostaDuplicato
		if !g.esiste {
			stato = db.StatoPropostaConfermata
		}
		decise := 0
		for _, m := range g.membri {
			if m.albero == nil || (g.esiste && m.albero.Ritrovato == nil && !m.rinominato) {
				continue
			}
			s := segno
			s.Commerciale = segnoCommercialeDi(g, m)
			for _, rr := range m.albero.Righe {
				if !rr.daDecidere() {
					continue
				}
				n, err := decidiNodoNellAlbero(ctx, q, rr.Proposta, stato, uuid.NullUUID{UUID: compDi[g], Valid: true}, "", s, utente, false)
				if err != nil {
					return "", err
				}
				if n > 0 {
					decise++
					stato = db.StatoPropostaDuplicato
				}
			}
		}
		es.righe += decise
		if g.esiste && decise > 0 {
			es.ritrovati++
		}
	}
	for _, n := range pc.via {
		for _, rr := range n.Righe {
			k, err := decidiNodoNellAlbero(ctx, q, rr.Proposta, db.StatoPropostaScartata, uuid.NullUUID{}, NotaToltoNellAlbero, segno, utente, true)
			if err != nil {
				return "", err
			}
			es.chiuse += int(k)
		}
	}
	for _, f := range pc.legami {
		stato := db.StatoPropostaDuplicato
		if scritto[f] {
			stato = db.StatoPropostaConfermata
		}
		s := segno
		s.Padre, s.Figlio = uid(compDi[f.padre]), uid(compDi[f.figlio])
		for _, l := range f.da {
			if l.albero == nil {
				continue
			}
			for _, fo := range l.albero.Fonti {
				if fo.Scartata {
					continue
				}
				nota := ""
				if fo.Qta != f.qta {
					nota = fmt.Sprintf("quantità decisa nell'albero: ×%d (il file dice ×%d)", f.qta, fo.Qta)
				}
				for _, rr := range fo.Righe {
					n, err := decidiArcoNellAlbero(ctx, q, thread, rr, stato, nota, s, utente, false)
					if err != nil {
						return "", err
					}
					es.righe += int(n)
				}
			}
		}
	}
	for i := range pc.a.Archi {
		l := &pc.a.Archi[i]
		if pc.usati[l] || l.Stato == StatoAlberoScartato {
			continue
		}
		for _, fo := range l.Fonti {
			if fo.Scartata {
				continue
			}
			for _, rr := range fo.Righe {
				n, err := decidiArcoNellAlbero(ctx, q, thread, rr, db.StatoPropostaScartata, NotaToltoNellAlbero, segno, utente, true)
				if err != nil {
					return "", err
				}
				es.chiuse += int(n)
			}
		}
	}

	// 7. i componenti che restano senza padri: archiviati con la loro storia, o eliminati
	for _, fu := range pc.fuori {
		if fu.Esito == FuoriArchivia {
			if _, err := ArchiviaComponente(ctx, q, thread, fu.Componente, utente, NotaToltoNellAlbero); err != nil {
				return "", err
			}
			es.archiviati = append(es.archiviati, fu.Codice)
			continue
		}
		if _, err := RimuoviComponente(ctx, q, thread, fu.Componente); err != nil {
			return "", err
		}
		es.eliminati = append(es.eliminati, fu.Codice)
	}

	// 8. le rimozioni ricalcolate; quelle che lo STEP autorizzato propone su un legame appena confermato si chiudono:
	// la persona l'ha tenuto (come l'editor, ApplicaStrutturaVoluta)
	msg, err := dopoLaDecisione(ctx, q, thread, fraseConferma(es), nil)
	if err != nil {
		return "", err
	}
	for _, f := range pc.legami {
		if err := tieniArcoMessoAMano(ctx, q, thread, Arco{Padre: compDi[f.padre], Figlio: compDi[f.figlio]}, utente); err != nil {
			return "", err
		}
	}
	return msg, nil
}

// righeDelLegame sono le righe degli STEP che fanno un legame finale: quelle delle fonti non scartate.
func righeDelLegame(f *legameFinale) []RigaArco {
	var out []RigaArco
	for _, l := range f.da {
		if l.albero == nil {
			continue
		}
		for _, fo := range l.albero.Fonti {
			if !fo.Scartata {
				out = append(out, fo.Righe...)
			}
		}
	}
	return out
}

// segnoCommercialeDi e' la risposta alla proposta commerciale del nodo, da scrivere nel segno: il ✓ (o la tendina)
// se il pezzo nasce commerciale per quella risposta, il ✗ se una persona l'ha dato. Una proposta decaduta non ha
// risposta.
func segnoCommercialeDi(g *gruppo, m *pezzoBozza) *SegnoCommerciale {
	if m.albero == nil || m.albero.Commerciale == nil {
		return nil
	}
	switch {
	case g.commercialeConfermato():
		return &SegnoCommerciale{Risposta: RispostaSi, Tendina: m.risposta == "", Motivo: m.albero.Commerciale.Motivo}
	case m.risposta == RispostaNo:
		return &SegnoCommerciale{Risposta: RispostaNo, Motivo: m.albero.Commerciale.Motivo}
	}
	return nil
}

// decidiNodoNellAlbero decide la riga di un nodo con il segno. 0: la riga era gia' decisa da una persona e non si
// tocca. anche: una riga che una decisione di prima aveva preso si decide di nuovo (un pezzo che la conferma toglie);
// una scartata da una persona resta com'e'.
func decidiNodoNellAlbero(ctx context.Context, q *db.Queries, proposta uuid.UUID, stato db.StatoProposta, comp uuid.NullUUID, nota string,
	s SegnoAlbero, utente uuid.UUID, anche bool) (int64, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return 0, err
	}
	return q.DecidiNodoNellAlbero(ctx, db.DecidiNodoNellAlberoParams{Stato: stato, ComponenteID: comp, DecisoDa: utente,
		Nota: testo(tagliaNota(nota)), Segno: raw, PropostaID: proposta, AncheDecise: anche})
}

// decidiArcoNellAlbero decide la riga di un arco con il segno, come decidiNodoNellAlbero.
func decidiArcoNellAlbero(ctx context.Context, q *db.Queries, thread uuid.UUID, rr RigaArco, stato db.StatoProposta, nota string,
	s SegnoAlbero, utente uuid.UUID, anche bool) (int64, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return 0, err
	}
	return q.DecidiArcoNellAlbero(ctx, db.DecidiArcoNellAlberoParams{Stato: stato, Nota: testo(tagliaNota(nota)), DecisoDa: utente,
		Segno: raw, ThreadID: thread, AllegatoID: rr.Allegato, PadreChiave: rr.Padre, FiglioChiave: rr.Figlio, AncheDecise: anche})
}

// fraseConferma dice all'operatore che cosa la conferma ha fatto.
func fraseConferma(es esitoConferma) string {
	var parti []string
	if es.nuovi > 0 {
		s := quanti(es.nuovi, "pezzo nuovo", "pezzi nuovi")
		if es.commerciali > 0 {
			s += " (" + quanti(es.commerciali, "particolare commerciale confermato", "particolari commerciali confermati") + ")"
		}
		parti = append(parti, s)
	}
	if es.ritrovati > 0 {
		parti = append(parti, quanti(es.ritrovati, "pezzo che c'era già", "pezzi che c'erano già"))
	}
	if es.ripristinati > 0 {
		parti = append(parti, quanti(es.ripristinati, "pezzo archiviato ripristinato", "pezzi archiviati ripristinati"))
	}
	if es.tipi > 0 {
		parti = append(parti, quanti(es.tipi, "tipo cambiato", "tipi cambiati"))
	}
	if es.revisioni > 0 {
		parti = append(parti, quanti(es.revisioni, "revisione nuova", "revisioni nuove"))
	}
	if es.legami > 0 {
		parti = append(parti, quanti(es.legami, "legame nuovo", "legami nuovi"))
	}
	if es.quantita > 0 {
		parti = append(parti, quanti(es.quantita, "quantità cambiata", "quantità cambiate"))
	}
	if es.tolti > 0 {
		parti = append(parti, quanti(es.tolti, "legame tolto", "legami tolti"))
	}
	if es.righe > 0 {
		parti = append(parti, quanti(es.righe, "riga degli STEP decisa", "righe degli STEP decise"))
	}
	if es.chiuse > 0 {
		parti = append(parti, quanti(es.chiuse, "riga degli STEP chiusa", "righe degli STEP chiuse"))
	}
	frase := "Albero confermato: niente da cambiare, le righe degli STEP erano già decise."
	if len(parti) > 0 {
		frase = "Albero confermato: " + strings.Join(parti, ", ") + "."
	}
	if len(es.archiviati) > 0 {
		frase += " Archiviati, senza padri: " + strings.Join(es.archiviati, ", ") + "."
	}
	if len(es.eliminati) > 0 {
		frase += " Eliminati, senza padri e senza storia: " + strings.Join(es.eliminati, ", ") + "."
	}
	if len(es.parti) > 0 {
		frase += " " + strings.Join(es.parti, " ")
	}
	return frase
}
