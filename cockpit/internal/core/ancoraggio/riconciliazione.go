package ancoraggio

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// La riconciliazione fra lo STEP e il cartiglio (piano A, 6.0.6; workflow, passo 10 e il caso del §4; contratto §0
// punto 4, §1.4, §2.2, §2.5, §2.6; commit P6b, B4, fase 3). Per ogni nodo di una struttura con un 2D associato (un
// candidato del file con una posizione sul nodo, o un'associazione decisa sul componente del nodo), il codice letto da
// cartiglio.codice dei fatti del 2D (oggi solo dai PDF: LD-04) si confronta con l'identità del nodo proposta dallo STEP,
// con ConfrontaBasi e ConfrontaRevisioni, mai le stringhe composte: concorda, completamento_proposto,
// correzione_proposta, discordante o non_verificabile (T-B0-11, T-B0-27; R64 A, R87). Il cartiglio è la fonte
// dell'identità letta dal documento, non una conferma (contratto §0 punto 4): niente si applica da solo, né il
// componente, né l'ancoraggio, né l'identità del nodo; il cartiglio aggiunge solo un candidato di revisione per la sua
// entità (T-E1-06). Accanto, mai fusa, la discordanza con la decisione del nodo, con le evidenze dei due lati
// (T-E1-15), che B5 e B6 compongono nel Conflitto: conflitto contro un codice deciso (T-B0-24) e contro una decisione
// tracciata solo con un'evidenza nuova (T-E1R-08, R95 A), indicatore contro componente.rev del legacy (R97 B). Sulla BOM
// di lavoro la radice scelta porta accanto il codice confermato del componente del target (T-B4-38), quindi anche il
// cartiglio del 2D del prodotto si confronta con la decisione.

// ---- gli ingressi ----

// AssociazioneDecisa: un'associazione di un file a un componente decisa nel DB (contratto §1.5), come ingresso già
// convertito da valutazione, che legge la fotografia (lettura T-B4-33):
//   - manuale: documento_proposta.componente_id su una proposta aperta («assegna»), senza chi né quando (LD-16);
//   - confermato: documento.componente_id con confermato_da, legato all'allegato da documento_provenienza: DocumentoID
//     c'è sempre.
//
// L'allegato è uno dei file dati a ProponiAncoraggi. Qui serve solo all'origine dell'associazione del codice
// documentale (CodiceDocumentale.OrigineAssociazione, DocumentoID): le associazioni, la pertinenza e lo smistamento le
// compone valutazione (B6), e un candidato resta un candidato.
type AssociazioneDecisa struct {
	AllegatoID   uuid.UUID   `json:"allegato_id"`
	DocumentoID  *uuid.UUID  `json:"documento_id,omitempty"`
	ComponenteID uuid.UUID   `json:"componente_id"`
	Origine      OrigineDato `json:"origine"`
}

// ---- il segnale della discordanza con la decisione ----

// DiscordanzaDecisione: il segnale della discordanza fra il codice documentale e la decisione del nodo (il codice
// confermato o manuale della catena), con le evidenze dei due lati (T-E1-15; lettura T-B4-34: campo in più di
// CodiceDocumentale). La decisione resta il valore corrente, la proposta le sta accanto (R95 A); il Conflitto composto,
// con l'asse della nomenclatura (T-E1R-08: «su un componente: conflitto di nomenclatura»), lo fanno B5 e B6.
//   - Parte: codice (la base o il marcatore del cartiglio contraddicono il codice deciso: R86) o revisione (lo stesso
//     codice, un'altra revisione; contro una decisione tracciata senza revisione anche una revisione qualunque, perché
//     l'assenza è decisa: T-B4-21).
//   - Effetto e Motivo: conflitto con evidenza_nuova (una decisione tracciata e una coppia (cartiglio, valore) che non
//     era fra le evidenze viste: T-E1R-08, R95 A); con codice_confermato (il codice confermato del legacy che il
//     cartiglio contraddice: T-B0-24, PO-23); con codice_manuale (il codice manuale di una riga aperta, T-B0-33, che il
//     cartiglio contraddice nel codice o nella revisione: l'ha scritto l'operatore, è una decisione, R95 A; T-B4-34
//     corretta); indicatore con evidenza_vista (lo stesso cartiglio che l'operatore aveva davanti: una correzione fatta
//     contro il cartiglio non è un conflitto permanente, E1R §5.3) o con revisione_registrata (solo componente.rev del
//     legacy, di provenienza non registrata: R97 B, T-E1R-07, mai un conflitto). La discordanza non si spegne mai
//     (R104): con l'evidenza vista resta, come indicatore.
//   - Decisione: il valore corrente con la sua origine e la provenienza della revisione (una copia del CodiceDeciso della
//     catena). DecisaDa e DecisaIl: chi e quando, solo per una decisione tracciata (il legacy non li ha: LD-13), in UTC
//     al millisecondo come nell'impronta (par.3.4.3).
//   - Evidenza: la coppia (cartiglio, testo grezzo) del valore proposto, costruita con EvidenzaDa (T-B4-22). Il valore
//     proposto, con l'allegato, l'unità e il localizzatore, è il codice documentale che porta il segnale.
//   - RevisioneInferiore: la revisione del cartiglio è minore di quella decisa (T-E1-20).
//
// Un 2D solo proposto con l'associazione del file ambigua o discordante può non essere del nodo: contro una decisione dà
// sempre un indicatore, mai un conflitto, qualunque sia il motivo (lettura T-B4-39). Con un'associazione decisa (manuale
// o confermata), o con un candidato unico proposto, l'effetto resta quello della tabella sopra (PO-23: un PDF nuovo
// contro una nomenclatura confermata).
type DiscordanzaDecisione struct {
	Parte              string        `json:"parte"`
	Effetto            string        `json:"effetto"`
	Motivo             string        `json:"motivo"`
	Decisione          CodiceDeciso  `json:"decisione"`
	DecisaDa           *uuid.UUID    `json:"decisa_da,omitempty"`
	DecisaIl           *time.Time    `json:"decisa_il,omitempty"`
	Evidenza           EvidenzaVista `json:"evidenza"`
	RevisioneInferiore bool          `json:"revisione_inferiore"`
}

// I valori di DiscordanzaDecisione.Parte, Effetto e Motivo (lettura T-B4-34). Parte usa le parole del Conflitto
// identita_documento (contratto §2.6: «la parte, codice o revisione»), Effetto quelle di DiscordanzaRevisione (§2.6).
const (
	ParteDiscordanzaCodice    = "codice"
	ParteDiscordanzaRevisione = "revisione"

	EffettoDiscordanzaConflitto  = "conflitto"
	EffettoDiscordanzaIndicatore = "indicatore"

	MotivoDiscordanzaEvidenzaNuova       = "evidenza_nuova"
	MotivoDiscordanzaEvidenzaVista       = "evidenza_vista"
	MotivoDiscordanzaCodiceConfermato    = "codice_confermato"
	MotivoDiscordanzaCodiceManuale       = "codice_manuale"
	MotivoDiscordanzaRevisioneRegistrata = "revisione_registrata"
)

// I motivi di un codice documentale (CodiceDocumentale.Motivo; lettura T-B4-35):
//   - non_verificabile: cartiglio_non_letto (il 2D non ha una lettura di cartiglio.codice: un raster, un PDF senza
//     testo, un cartiglio senza codice o che la grammatica non legge: LD-04, R87); nodo_senza_identita (il nodo non ha
//     una lettura scelta: niente con cui confrontare); cartiglio_altro_namespace (il cartiglio si legge solo in un altro
//     spazio di codici); cartiglio_non_completo (la lettura del cartiglio non è un codice intero: A-C07, D2);
//     base_non_confrontabile (ConfrontaBasi non lo determina); revisione_cartiglio_assente (base e marcatore
//     concordano, lo STEP dà la revisione e il cartiglio no); revisione_cartiglio_ambigua (la revisione del cartiglio
//     c'è ma non è letta: D5);
//   - discordante: letture_discordanti (le letture del cartiglio non danno la stessa identità: nessuna si sceglie,
//     T-E1-07) o identita_discordanti (il file ha identità che non concordano, nome e cartiglio anche solo nella
//     revisione, o una base ripetuta che non concorda: l'associazione è discordante, R64 A, T-B0-35).
//
// concorda, completamento_proposto e correzione_proposta non hanno motivo.
const (
	MotivoDocumentaleCartiglioNonLetto    = "cartiglio_non_letto"
	MotivoDocumentaleNodoSenzaIdentita    = "nodo_senza_identita"
	MotivoDocumentaleAltroNamespace       = "cartiglio_altro_namespace"
	MotivoDocumentaleCartiglioNonCompleto = "cartiglio_non_completo"
	MotivoDocumentaleBaseNonConfrontabile = "base_non_confrontabile"
	MotivoDocumentaleRevisioneAssente     = "revisione_cartiglio_assente"
	MotivoDocumentaleRevisioneAmbigua     = "revisione_cartiglio_ambigua"
)

// campoCartiglioCodice: il campo del cartiglio che dà il codice documentale (contratto §1.4: «cartiglio.codice dai
// fatti del 2D associato»). Il numero di disegno e la revisione in un campo a sé non sono il codice documentale.
const campoCartiglioCodice = "codice"

// ---- la riconciliazione ----

// riconciliatore: ciò che serve alla riconciliazione delle strutture dell'esito. Le mappe servono solo a cercare,
// mai a scorrere.
type riconciliatore struct {
	c            *contestoIndicizzato
	disegni      map[uuid.UUID]*FileInterpretato    // i 2D, per allegato
	associazioni map[uuid.UUID]Associazione         // l'associazione dell'ancoraggio di ogni 2D
	proposti     map[string][]uuid.UUID             // per posizione (target, allegato e radice della struttura, nodo): i 2D candidati
	decise       map[uuid.UUID][]AssociazioneDecisa // per componente, in ordine canonico
	target       map[string]ProdottoRichiesto
}

// associato: un 2D associato a un nodo, con l'origine più forte dell'associazione (confermato, poi manuale, poi
// proposto: le decisioni prevalgono sulla proposta, workflow passo 11) e il documento, se l'associazione è decisa.
type associato struct {
	allegato  uuid.UUID
	origine   OrigineDato
	documento *uuid.UUID
}

// riconcilia (B4, fase 3): la riconciliazione delle strutture dell'esito, dopo gli ancoraggi. Per ogni nodo con dei 2D
// associati, un CodiceDocumentale per 2D, in ordine di allegato, i candidati di revisione del cartiglio con l'entità del
// nodo e lo stato della revisione ricalcolato; per ogni codice documentale con una correzione accanto, la diagnostica
// ancoraggio.completamento_documentale o ancoraggio.correzione_documentale. Cambia solo le strutture dell'esito, che
// ProponiStrutture ha appena costruito.
func riconcilia(strutture []StrutturaProdotto, ancoraggi []AncoraggioFile, file []FileInterpretato, target []ProdottoRichiesto, ctx ContestoStrutturale) []evidenze.Diagnostica {
	r := &riconciliatore{c: indicizza(ctx), disegni: map[uuid.UUID]*FileInterpretato{}, associazioni: map[uuid.UUID]Associazione{},
		proposti: map[string][]uuid.UUID{}, decise: map[uuid.UUID][]AssociazioneDecisa{}, target: map[string]ProdottoRichiesto{}}
	for i := range file {
		if file[i].Disegno {
			r.disegni[file[i].AllegatoID] = &file[i]
		}
	}
	for _, a := range ancoraggi {
		if r.disegni[a.AllegatoID] == nil {
			continue
		}
		r.associazioni[a.AllegatoID] = a.Associazione
		for _, c := range a.Candidati {
			for _, p := range c.Posizioni {
				k := chiavePosizioneCandidato(p)
				r.proposti[k] = append(r.proposti[k], a.AllegatoID)
			}
		}
	}
	decise := append([]AssociazioneDecisa(nil), ctx.AssociazioniDecise...)
	sort.SliceStable(decise, func(i, j int) bool { return chiaveAssociazioneDecisa(decise[i]) < chiaveAssociazioneDecisa(decise[j]) })
	for _, x := range decise {
		if r.disegni[x.AllegatoID] != nil {
			r.decise[x.ComponenteID] = append(r.decise[x.ComponenteID], x)
		}
	}
	for _, t := range target {
		r.target[t.Rif] = t
	}

	var diag []evidenze.Diagnostica
	for si := range strutture {
		s := &strutture[si]
		t := r.target[s.Target]
		for ni := range s.Nodi {
			n := &s.Nodi[ni]
			associati := r.associati(s, n, t)
			if len(associati) == 0 {
				continue
			}
			x := nodoDellaCatena{t: t, s: s, n: n}
			id := &n.Codice.Identita
			cand := append([]CandidatoRevisione(nil), id.CandidatiRevisione...)
			senza := append([]FonteSenzaRevisione(nil), id.FontiSenzaRevisione...)
			var documentali []CodiceDocumentale
			for _, a := range associati {
				cd, sotto, c, f := r.documentale(x, a)
				documentali = append(documentali, cd)
				cand, senza = append(cand, c...), append(senza, f...)
				if cd.Correzione != nil {
					diag = append(diag, diagnosticaDocumentale(s, n, cd, sotto))
				}
			}
			n.Codice.Documentale = documentali
			id.CandidatiRevisione, id.FontiSenzaRevisione = ordinaCandidatiRevisione(cand), ordinaSenzaRevisione(senza)
			id.StatoRevisione = statoRevisione(n.Codice)
		}
	}
	return diag
}

// chiaveAssociazioneDecisa: l'ordine canonico e la chiave dei doppioni di un'associazione decisa.
func chiaveAssociazioneDecisa(a AssociazioneDecisa) string {
	return a.AllegatoID.String() + "\x00" + a.ComponenteID.String() + "\x00" + string(a.Origine) + "\x00" + testoUUID(a.DocumentoID)
}

// rangoOrigineAssociazione: proposto, poi manuale, poi confermato (workflow, passo 11: manuale e confermato prevalgono
// sempre su proposto).
func rangoOrigineAssociazione(o OrigineDato) int {
	switch o {
	case OrigineConfermato:
		return 3
	case OrigineManuale:
		return 2
	case OrigineProposto:
		return 1
	}
	return 0
}

// componenteDelNodo: il componente a cui un'associazione decisa lega i 2D del nodo: la decisione per UUID del nodo
// (NodoProposto.Decisione); per la radice di una struttura che rappresenta il prodotto (la sua base o la radice scelta
// con il gesto 3), senza una decisione propria, il componente del target: i documenti del prodotto sono quelli della
// sua radice (lettura T-B4-37, come per gli archi della radice scelta, T-B4-23). La decisione della radice resta nil:
// questo legame vale solo per l'origine dell'associazione; il codice del prodotto accanto alla radice lo porta la catena,
// e solo sulla BOM di lavoro (T-B4-38). Altrimenti nessuno: un 2D deciso su un componente non si lega a un nodo senza
// decisione.
func componenteDelNodo(s *StrutturaProdotto, n *NodoProposto, t ProdottoRichiesto) *uuid.UUID {
	switch {
	case n.Decisione != nil:
		id := n.Decisione.ComponenteID
		return &id
	case n.Rif == s.Radice && radiceProdotto(*s) && t.ComponenteID != nil:
		return copiaUUID(t.ComponenteID)
	}
	return nil
}

// associati: i 2D associati al nodo nella struttura, in ordine di allegato: i candidati con una posizione sul nodo
// (l'associazione proposta della fase 2) e le associazioni decise sul componente del nodo; un 2D in più modi ha l'origine
// più forte, con il documento dell'associazione che la dà (a parità il documento minore).
func (r *riconciliatore) associati(s *StrutturaProdotto, n *NodoProposto, t ProdottoRichiesto) []associato {
	per := map[uuid.UUID]*associato{}
	var ordine []uuid.UUID
	aggiungi := func(allegato uuid.UUID, origine OrigineDato, documento *uuid.UUID) {
		a, ok := per[allegato]
		if !ok {
			a = &associato{allegato: allegato}
			per[allegato] = a
			ordine = append(ordine, allegato)
		}
		switch ra, ro := rangoOrigineAssociazione(a.origine), rangoOrigineAssociazione(origine); {
		case ro > ra:
			a.origine, a.documento = origine, copiaUUID(documento)
		case ro == ra && documento != nil && (a.documento == nil || documento.String() < a.documento.String()):
			a.documento = copiaUUID(documento)
		}
	}
	k := chiavePosizioneCandidato(PosizioneCandidato{Target: s.Target, AllegatoID: s.AllegatoID, Radice: s.Radice, Nodo: n.Rif})
	for _, allegato := range r.proposti[k] {
		aggiungi(allegato, OrigineProposto, nil)
	}
	if comp := componenteDelNodo(s, n, t); comp != nil {
		for _, x := range r.decise[*comp] {
			aggiungi(x.AllegatoID, x.Origine, x.DocumentoID)
		}
	}
	sort.Slice(ordine, func(i, j int) bool { return ordine[i].String() < ordine[j].String() })
	out := make([]associato, 0, len(ordine))
	for _, a := range ordine {
		out = append(out, *per[a])
	}
	return out
}

// documentale: il codice documentale di un 2D associato al nodo, con l'esito, la correzione accanto e le discordanze
// con le decisioni del nodo; l'esito del confronto con lo STEP prima che il file discordante lo renda discordante (per
// la diagnostica); i candidati di revisione e le fonti senza revisione del cartiglio, con l'entità del nodo (T-E1-06).
//
//  1. Le letture di cartiglio.codice del 2D. Nessuna: non_verificabile, cartiglio_non_letto (un raster o un PDF senza
//     testo: LD-04; resta la nomenclatura proposta dallo STEP, R87).
//  2. Senza un'identità del nodo, o con il cartiglio letto solo in un altro namespace: non_verificabile, con il motivo;
//     la lettura del cartiglio si mostra se è una sola identità.
//  3. Le letture nel namespace del nodo danno più identità: discordante, letture_discordanti, nessuna scelta (T-E1-07).
//     Altrimenti vale la lettura scelta come per i nodi (T-B4-01: la stessa identità, l'ID minore).
//  4. Il confronto con l'identità del nodo (confrontaConLoSTEP). Una base ripetuta che non concorda (D2) non è un
//     codice intero: nessuna correzione e nessuna discordanza con la decisione, perché la prima base non si sceglie.
//  5. Con il file discordante (identità che non concordano, nome e cartiglio anche solo nella revisione, o una base
//     ripetuta che non concorda: R64 A, T-B0-35, D2) l'esito è discordante, e la correzione, se c'è, resta accanto.
//
// Il codice documentale e la correzione non cambiano niente della catena: niente si applica da solo (R64 A, R87).
func (r *riconciliatore) documentale(x nodoDellaCatena, a associato) (CodiceDocumentale, EsitoRiconciliazione, []CandidatoRevisione, []FonteSenzaRevisione) {
	f := r.disegni[a.allegato]
	cd := CodiceDocumentale{AllegatoID: a.allegato, DocumentoID: copiaUUID(a.documento), OrigineAssociazione: a.origine,
		Associazione: r.associazioni[a.allegato]}
	unita := make(map[string]evidenze.UnitaEvidenza, len(f.Documento.Unita))
	for _, u := range f.Documento.Unita {
		unita[u.ID] = u
	}
	cart := lettureCartiglio(f.Interpretazione)
	if len(cart) == 0 {
		cd.Esito, cd.Motivo = RiconciliazioneNonVerificabile, MotivoDocumentaleCartiglioNonLetto
		return cd, cd.Esito, nil, nil
	}

	forma := x.n.Codice.Forma
	var scelta *motorea.LetturaCodice
	var cand []CandidatoRevisione
	var senza []FonteSenzaRevisione
	sotto := RiconciliazioneNonVerificabile
	switch pool := nelNamespace(cart, forma); {
	case forma == nil:
		scelta, _ = letturaDelNodo(cart)
		cd.Esito, cd.Motivo = RiconciliazioneNonVerificabile, MotivoDocumentaleNodoSenzaIdentita
	case len(pool) == 0:
		scelta, _ = letturaDelNodo(cart)
		cd.Esito, cd.Motivo = RiconciliazioneNonVerificabile, MotivoDocumentaleAltroNamespace
	default:
		cand, senza = r.candidatiDelCartiglio(x, *forma, pool, f.AllegatoID, unita)
		l, motivo := letturaDelNodo(pool)
		if l == nil {
			cd.Esito, cd.Motivo, sotto = RiconciliazioneDiscordante, motivo, RiconciliazioneDiscordante
			break
		}
		scelta = l
		sotto, cd.Motivo = confrontaConLoSTEP(*forma, l.Forma)
		cd.Esito = sotto
		if sotto == RiconciliazioneCompletamentoProposto || sotto == RiconciliazioneCorrezioneProposta {
			cd.Correzione = correzioneDa(*l, f.AllegatoID, unita[l.UnitaID])
		}
		if cd.Associazione == AssociazioneDiscordante {
			cd.Esito, cd.Motivo = RiconciliazioneDiscordante, MotivoAncoraggioIdentitaDiscordanti
		}
	}
	if scelta != nil {
		u := unita[scelta.UnitaID]
		cd.Lettura, cd.Originale, cd.Base, cd.Revisione = scelta.ID, u.Testo, copiaBase(scelta.Forma.Base), copiaRevisione(scelta.Forma.Revisione)
		cd.UnitaID, cd.Posizione = scelta.UnitaID, copiaLocalizzatore(u.Posizione)
		if !ripetizioneDiscorde(scelta.Forma) {
			var inferiore bool
			cd.Discordanze, inferiore = r.discordanze(x.n, cd, scelta.Forma, u.Testo)
			if cd.Correzione != nil {
				cd.Correzione.RevisioneInferiore = inferiore
			}
		}
	}
	return cd, sotto, cand, senza
}

// lettureCartiglio: le letture d'identità di cartiglio.codice di un file, in ordine di ID.
func lettureCartiglio(r motorea.Interpretazione) []motorea.LetturaCodice {
	var out []motorea.LetturaCodice
	for _, l := range lettureIdentita(r) {
		if s := l.Forma.Selettore; s.Contesto == evidenze.ContestoCartiglio && s.Campo.Valore == campoCartiglioCodice {
			out = append(out, l)
		}
	}
	return out
}

// nelNamespace: le letture nel namespace dell'identità del nodo; nessuna senza identità.
func nelNamespace(letture []motorea.LetturaCodice, forma *motorea.LetturaForma) []motorea.LetturaCodice {
	if forma == nil {
		return nil
	}
	var out []motorea.LetturaCodice
	for _, l := range letture {
		if l.Forma.Namespace == forma.Namespace {
			out = append(out, l)
		}
	}
	return out
}

// ripetizioneDiscorde: la lettura ha una ripetizione della base che non concorda (D2): nessuna scelta della prima.
func ripetizioneDiscorde(f motorea.LetturaForma) bool {
	for _, r := range f.Ripetizioni {
		if !r.Concorda {
			return true
		}
	}
	return false
}

// confrontaConLoSTEP: il confronto del cartiglio con l'identità del nodo proposta dallo STEP (workflow, passo 10;
// T-B0-11, T-B0-27; R86, R87), con ConfrontaBasi, la regola unica del marcatore (T-B4-06 rivisto, T-B4-30) e
// ConfrontaRevisioni, mai le stringhe composte:
//   - una lettura del cartiglio che non è un codice intero: non_verificabile (non propone niente);
//   - basi non confrontabili: non_verificabile; basi discordanti, la base del nodo parziale che il cartiglio completa,
//     o due marcatori scritti e diversi: correzione_proposta (il codice documentale è diverso);
//   - base e marcatore concordano: la stessa revisione letta (uguale o equivalente dichiarata) o nessuna da tutte e
//     due le parti, concorda; una revisione diversa, correzione_proposta; il cartiglio aggiunge la revisione che lo STEP
//     non dà (anche quando lo STEP la porta non letta), completamento_proposto (R87, T-B0-27); lo STEP la dà e il
//     cartiglio no, o il cartiglio la porta non letta: non_verificabile.
func confrontaConLoSTEP(nodo, cart motorea.LetturaForma) (EsitoRiconciliazione, string) {
	if letturaNonCompleta(cart) {
		return RiconciliazioneNonVerificabile, MotivoDocumentaleCartiglioNonCompleto
	}
	switch b := motorea.ConfrontaBasi(nodo.Base, cart.Base); {
	case b == motorea.CompatibilitaNonDeterminabile:
		return RiconciliazioneNonVerificabile, MotivoDocumentaleBaseNonConfrontabile
	case b == motorea.CompatibilitaDiscordante, b == motorea.CompatibilitaParziale, !marcatoriCompatibili(marcatoreDi(nodo), marcatoreDi(cart)):
		return RiconciliazioneCorrezioneProposta, ""
	}
	rn, rc := revisioneDi(nodo), revisioneDi(cart)
	switch {
	case rc == nil && cart.Revisione != nil:
		return RiconciliazioneNonVerificabile, MotivoDocumentaleRevisioneAmbigua
	case rc == nil && rn != nil:
		return RiconciliazioneNonVerificabile, MotivoDocumentaleRevisioneAssente
	case rc == nil:
		return RiconciliazioneConcorda, ""
	case rn == nil:
		return RiconciliazioneCompletamentoProposto, ""
	}
	if v := motorea.ConfrontaRevisioni(*rn, *rc); v == motorea.CompatibilitaUguale || v == motorea.CompatibilitaEquivalente {
		return RiconciliazioneConcorda, ""
	}
	return RiconciliazioneCorrezioneProposta, ""
}

// correzioneDa: la correzione o il completamento proposti dal cartiglio, con la provenienza (il file, l'unità, il
// localizzatore: 6.0.6): il codice com'è letto, la base, la revisione letta. Mai applicati (R64 A, R87).
func correzioneDa(l motorea.LetturaCodice, allegato uuid.UUID, u evidenze.UnitaEvidenza) *CorrezioneProposta {
	c := &CorrezioneProposta{Codice: l.Forma.Originale, Base: copiaBase(l.Forma.Base), AllegatoID: allegato, UnitaID: l.UnitaID,
		Posizione: copiaLocalizzatore(u.Posizione)}
	if r, ok := revisioneLetta(l.Forma); ok {
		c.Revisione = &r
	}
	return c
}

// candidatiDelCartiglio: il cartiglio di un 2D associato come indizio di revisione per l'entità del nodo (T-E1-06:
// «il cartiglio di un 2D associato, per il suo componente»), con la regola unica di confrontaIdentita (T-B4-06
// rivisto): una lettura con la stessa identità e la revisione letta è un candidato; senza revisione letta, una fonte
// senza revisione con il motivo; con lo stesso namespace e la stessa base e un altro marcatore scritto, marcatore_diverso.
// Una lettura con un'altra base non è un indizio di revisione: è una correzione (il codice documentale). Una base
// ripetuta che non concorda (D2) non dà indizi. Il candidato non decide niente e non passa ai figli.
func (r *riconciliatore) candidatiDelCartiglio(x nodoDellaCatena, forma motorea.LetturaForma, letture []motorea.LetturaCodice, allegato uuid.UUID,
	unita map[string]evidenze.UnitaEvidenza) ([]CandidatoRevisione, []FonteSenzaRevisione) {
	entita := entitaDelNodo(x)
	var cand []CandidatoRevisione
	var senza []FonteSenzaRevisione
	for _, l := range letture {
		if ripetizioneDiscorde(l.Forma) {
			continue
		}
		motivo := ""
		switch confrontaIdentita(forma, l.Forma) {
		case identitaDiversa:
			continue
		case identitaMarcatoreDiverso:
			motivo = MotivoSenzaRevisioneMarcatoreDiverso
		default:
			if v, ok := revisioneLetta(l.Forma); ok {
				cand = append(cand, CandidatoRevisione{Valore: v, Fonte: FonteRevisioneCartiglio, AllegatoID: copiaUUID(&allegato),
					Posizione: copiaLocalizzatore(unita[l.UnitaID].Posizione), Entita: entita})
				continue
			}
			motivo = MotivoSenzaRevisioneNonLetta
			if l.Forma.Revisione != nil {
				motivo = MotivoSenzaRevisioneAmbigua
			}
		}
		senza = append(senza, FonteSenzaRevisione{Fonte: FonteRevisioneCartiglio, Entita: entita, AllegatoID: copiaUUID(&allegato), Motivo: motivo})
	}
	return cand, senza
}

// discordanze: i segnali della discordanza fra la lettura del cartiglio e le decisioni del nodo, uno per decisione
// contraddetta, in ordine di origine (il codice confermato, anche quello del componente del target sulla radice scelta
// della BOM di lavoro, T-B4-38; poi il codice manuale della riga aperta): sono due decisioni, e nessuna precedenza ne
// nasconde una (lettura T-B4-40). Inferiore: la revisione del cartiglio è minore di almeno una revisione decisa
// (T-E1-20). Un 2D solo proposto con l'associazione del file ambigua o discordante dà solo indicatori (T-B4-39).
func (r *riconciliatore) discordanze(n *NodoProposto, cd CodiceDocumentale, cart motorea.LetturaForma, testo string) ([]DiscordanzaDecisione, bool) {
	type deciso struct {
		dec *CodiceDeciso
		ld  *motorea.LetturaForma
	}
	var decisi []deciso
	if dec := n.Codice.Confermato; dec != nil {
		// Il componente della decisione: quello del nodo, o sulla BOM di lavoro quello del target per la radice scelta
		// (T-B4-38). È sempre fra i componenti del contesto, perché la catena lo prende da lì.
		x := deciso{dec: dec}
		if k, ok := r.c.componenti[*dec.ComponenteID]; ok {
			x.ld = r.c.letturaDecisa(&k)
		}
		decisi = append(decisi, x)
	}
	if n.Codice.Manuale != nil && n.RigaLegacy != nil {
		decisi = append(decisi, deciso{dec: n.Codice.Manuale, ld: n.RigaLegacy.LetturaManuale})
	}
	incerta := cd.OrigineAssociazione == OrigineProposto && (cd.Associazione == AssociazioneAmbiguo || cd.Associazione == AssociazioneDiscordante)
	var out []DiscordanzaDecisione
	inferiore := false
	for _, x := range decisi {
		d, inf := r.discordanza(x.dec, x.ld, cart, testo)
		inferiore = inferiore || inf
		if d == nil {
			continue
		}
		if incerta {
			d.Effetto = EffettoDiscordanzaIndicatore
		}
		out = append(out, *d)
	}
	return out, inferiore
}

// discordanza: il segnale della discordanza fra la lettura del cartiglio e una decisione del nodo, e se la revisione del
// cartiglio è minore di quella decisa (T-E1-20). Il codice si confronta
// con la lettura del codice deciso (ComponenteDeciso.Lettura, LettureDecise, LetturaManuale) con ConfrontaBasi e la
// regola unica del marcatore; senza quella lettura, o in un altro namespace, il codice non si confronta e resta la
// revisione. La revisione decisa è il valore registrato, che la grammatica non legge: si confronta, senza gli spazi ai
// bordi, con la revisione letta del cartiglio (nessuna equivalenza è dichiarata in A1: P1 §5.2). Una revisione decisa
// assente non discorda, salvo con una decisione tracciata, perché lì l'assenza è decisa (T-B4-21). Niente che discordi:
// nessun segnale.
func (r *riconciliatore) discordanza(dec *CodiceDeciso, ld *motorea.LetturaForma, cart motorea.LetturaForma, testo string) (*DiscordanzaDecisione, bool) {
	tracciata := dec.RevProvenienza == RevProvenienzaDecisioneTracciata
	codice := motorea.CompatibilitaNonDeterminabile
	if ld != nil && ld.Namespace == cart.Namespace {
		codice = conMarcatore(motorea.ConfrontaBasi(ld.Base, cart.Base), marcatoreDi(*ld), marcatoreDi(cart))
	}
	rc := revisioneDi(cart)
	inferiore := codice != motorea.CompatibilitaDiscordante && rc != nil && revisioneInferiore(dec.Rev, *rc)

	parte := ""
	switch {
	case codice == motorea.CompatibilitaDiscordante:
		parte = ParteDiscordanzaCodice
	case rc == nil:
	case conTesto(dec.Rev):
		if strings.TrimSpace(*dec.Rev) != rc.Normalizzata {
			parte = ParteDiscordanzaRevisione
		}
	case tracciata:
		parte = ParteDiscordanzaRevisione
	}
	if parte == "" {
		return nil, inferiore
	}

	d := &DiscordanzaDecisione{Parte: parte, Decisione: copiaCodiceDeciso(*dec), Evidenza: EvidenzaDa(FonteEvidenzaCartiglio, testo), RevisioneInferiore: inferiore}
	switch {
	case tracciata:
		gesto := r.c.decisioniComponente[*dec.ComponenteID]
		da, il := gesto.Da, gesto.Il.UTC().Truncate(time.Millisecond)
		d.DecisaDa, d.DecisaIl = &da, &il
		d.Effetto, d.Motivo = EffettoDiscordanzaIndicatore, MotivoDiscordanzaEvidenzaVista
		if EvidenzaNuova(gesto, d.Evidenza) {
			d.Effetto, d.Motivo = EffettoDiscordanzaConflitto, MotivoDiscordanzaEvidenzaNuova
		}
	case dec.Origine == OrigineManuale:
		d.Effetto, d.Motivo = EffettoDiscordanzaConflitto, MotivoDiscordanzaCodiceManuale
	case parte == ParteDiscordanzaRevisione:
		d.Effetto, d.Motivo = EffettoDiscordanzaIndicatore, MotivoDiscordanzaRevisioneRegistrata
	default:
		d.Effetto, d.Motivo = EffettoDiscordanzaConflitto, MotivoDiscordanzaCodiceConfermato
	}
	return d, inferiore
}

// letturaDecisa: la lettura del codice deciso di un componente confermato: con una DecisioneIdentita sul componente la
// lettura che valutazione mette in LettureDecise (T-B4-32), altrimenti quella del componente solo se il codice deciso è
// lo stesso, altrimenti nessuna; senza decisione tracciata la lettura del componente (R31 c).
func (c *contestoIndicizzato) letturaDecisa(d *ComponenteDeciso) *motorea.LetturaForma {
	if dec, ok := c.decisioniComponente[d.ComponenteID]; ok {
		if l, ok := c.lettureDecise[RifComponente(d.ComponenteID)]; ok {
			return &l
		}
		if dec.Codice != d.Codice {
			return nil
		}
	}
	return d.Lettura
}

// revisioneInferiore (T-E1-20): la revisione proposta è minore di quella decisa. Le revisioni non si ordinano in
// generale (motorea non le ordina, P1 §5.2; R97: progressiva, mai «+1»): si dice minore solo quando l'ordine è certo,
// cioè quando tutte e due sono numeri interi scritti con le sole cifre (la proposta letta in un segmento solo), e il
// numero della proposta è minore; gli zeri a sinistra non contano. Altrimenti falso: non determinabile non vuol dire
// minore (lettura T-B4-36).
func revisioneInferiore(decisa *string, proposta motorea.RevisioneLetta) bool {
	if decisa == nil || proposta.Stato != motorea.StatoRevisioneLetta || len(proposta.Segmenti) > 1 {
		return false
	}
	a, b := strings.TrimSpace(*decisa), proposta.Normalizzata
	if !soloCifre(a) || !soloCifre(b) {
		return false
	}
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return len(b) < len(a)
	}
	return b < a
}

// soloCifre: un testo non vuoto fatto solo di cifre ASCII.
func soloCifre(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// statoRevisione: lo stato della revisione dell'identità di un nodo (contratto §2.5, E1R): confermata solo con una
// decisione tracciata sul componente; candidata con una revisione letta o un candidato (anche del cartiglio); assente
// altrimenti. componente.rev del legacy non dà mai confermata (R97 B).
func statoRevisione(cat CatenaCodice) string {
	switch {
	case cat.Confermato != nil && cat.Confermato.RevProvenienza == RevProvenienzaDecisioneTracciata:
		return StatoRevisioneConfermata
	case cat.Identita.Revisione != nil || len(cat.Identita.CandidatiRevisione) > 0:
		return StatoRevisioneCandidata
	}
	return StatoRevisioneAssente
}

// diagnosticaDocumentale: la diagnostica di un codice documentale con la correzione accanto (R41 b), in avviso, con
// il codice del confronto con lo STEP: completamento_documentale quando il cartiglio aggiunge solo la revisione (R87),
// correzione_documentale altrimenti (R64), anche per il file discordante con la correzione accanto.
func diagnosticaDocumentale(s *StrutturaProdotto, n *NodoProposto, cd CodiceDocumentale, sotto EsitoRiconciliazione) evidenze.Diagnostica {
	d := evidenze.Diagnostica{Codice: CodiceCorrezioneDocumentale, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
		Percorso: percorsoStrutture + "[" + s.Target + "].nodi[" + n.Rif + "].codice.documentale",
		Rif:      []string{s.Target, idFonteAllegato(s.AllegatoID), n.Rif, idFonteAllegato(cd.AllegatoID)}}
	if sotto == RiconciliazioneCompletamentoProposto {
		d.Codice = CodiceCompletamentoDocumentale
		d.Messaggio = fmt.Sprintf("il cartiglio del 2D ha la base e il marcatore dell'identità proposta dallo STEP e aggiunge la revisione «%s»: un completamento proposto, mai applicato; la nomenclatura la conferma l'operatore (R87)",
			testoP(cd.Correzione.Revisione))
	} else {
		d.Messaggio = fmt.Sprintf("il cartiglio del 2D dice «%s», un codice diverso dall'identità proposta dallo STEP: una correzione proposta, mai applicata; il componente e l'ancoraggio non cambiano (R64 A)",
			cd.Correzione.Codice)
	}
	if cd.Esito == RiconciliazioneDiscordante {
		d.Messaggio += "; il file ha identità che non concordano, e l'associazione resta discordante (T-B0-35)"
	}
	return d
}

// testoP: il testo di un puntatore; "" se nil.
func testoP(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ---- le copie ----

// copiaRevisione: la revisione letta con i segmenti e il token copiati. Le equivalenze dichiarate sono un campo privato
// di motorea, che nessuno fuori da lì può cambiare: restano condivise (come in copiaLetturaForma).
func copiaRevisione(r *motorea.RevisioneLetta) *motorea.RevisioneLetta {
	if r == nil {
		return nil
	}
	v := *r
	v.Segmenti = append([]motorea.SegmentoLetto(nil), r.Segmenti...)
	if len(v.Segmenti) == 0 {
		v.Segmenti = nil
	}
	v.Token = copiaParte(r.Token)
	return &v
}

// copiaCodiceDeciso: il codice deciso con i puntatori e la base copiati.
func copiaCodiceDeciso(d CodiceDeciso) CodiceDeciso {
	d.ComponenteID, d.Rev, d.Base = copiaUUID(d.ComponenteID), copiaTesto(d.Rev), copiaBase(d.Base)
	return d
}
