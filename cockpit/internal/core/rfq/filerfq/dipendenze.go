package filerfq

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Gli esiti di una dipendenza (EM §10.3).
const (
	EsitoBlocca      = "blocca"
	EsitoRisoluzione = "risoluzione"
	EsitoMostra      = "mostra"
)

// Le risoluzioni dichiarate nel corpo dei gesti: una per tipo, un booleano (giro 2, NV-01).
const (
	RisoluzioneScollega   = "scollega"
	RisoluzioneUnicaCopia = "conferma_unica_copia"
)

// Dipendenza e' una voce dell'elenco delle dipendenze di un'occorrenza del gesto (EM §10.3; PE-06): l'occorrenza a cui
// si riferisce, il codice (D-1…D-15, D-6b, D-7b), l'esito, la risoluzione che chiede, il testo per chi guarda e un
// identificativo per l'ordine e l'impronta (un documento, un componente, una bozza, un tipo di lavoro, un contesto).
type Dipendenza struct {
	AllegatoID  uuid.UUID `json:"allegato_id"`
	Codice      string    `json:"codice"`
	Esito       string    `json:"esito"`
	Risoluzione string    `json:"risoluzione,omitempty"`
	Testo       string    `json:"testo"`
	Rif         string    `json:"rif,omitempty"`
}

// ElencoDipendenze e' l'elenco completo delle dipendenze delle occorrenze di un gesto, ordinato per allegato_id,
// codice e identificativo, con la sua impronta (EM §10.3: «impronta_dipendenze»).
//
// [T] della corsia file: l'elenco non dipende dal gesto. E' quello di «Scarta» (il piu' largo: comprende D-9, D-12,
// D-14), e il gesto ne applica la sua parte: i blocchi (D-1, D-3, D-6) valgono per tutti e due, la risoluzione
// «scollega» (D-7) per tutti e due, la «conferma dell'unica copia» (D-14) solo per «Scarta» (con «Altri file» i byte si
// conservano). Cosi' il dettaglio di S1 ha un elenco e un'impronta soli, e il gesto li ricalcola uguali.
type ElencoDipendenze struct {
	Voci     []Dipendenza
	Impronta string
}

// calcolaDipendenze calcola l'elenco delle dipendenze per le occorrenze del gesto (l'occorrenza del percorso e le voci
// del perimetro), con la stessa funzione nel dettaglio di S1 (fotografia in sola lettura, senza lucchetto) e nel gesto
// (sotto BloccaThread; giro 1, TR-08). nVoci: per un archivio, il numero delle voci del perimetro (D-10).
func (s *Servizio) calcolaDipendenze(ctx context.Context, q *db.Queries, thread uuid.UUID, occ []Occorrenza,
	nVoci map[uuid.UUID]int) (ElencoDipendenze, error) {
	ids := idDi(occ)
	nel := map[uuid.UUID]bool{}
	for _, id := range ids {
		nel[id] = true
	}
	var voci []Dipendenza
	righe, err := q.ListDipendenzeFile(ctx, db.ListDipendenzeFileParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return ElencoDipendenze{}, fmt.Errorf("dipendenze dei file: %w", err)
	}
	for _, r := range righe {
		voci = append(voci, dipendenzaDaRiga(r))
	}
	// D-6 e D-6b: le bozze aperte della RFQ che citano le occorrenze
	bozze, err := q.ListDipendenzeBozzaPerFile(ctx, thread)
	if err != nil {
		return ElencoDipendenze{}, fmt.Errorf("dipendenze dalle bozze: %w", err)
	}
	var prodotti []uuid.UUID
	vistoProdotto := map[uuid.UUID]bool{}
	for _, b := range bozze {
		if nel[b.AllegatoID] && !vistoProdotto[b.ProdottoID] {
			vistoProdotto[b.ProdottoID] = true
			prodotti = append(prodotti, b.ProdottoID)
		}
	}
	codici := map[uuid.UUID]string{}
	if len(prodotti) > 0 {
		cc, err := q.ListCodiciComponenti(ctx, db.ListCodiciComponentiParams{ThreadID: thread, Componenti: prodotti})
		if err != nil {
			return ElencoDipendenze{}, err
		}
		for _, c := range cc {
			codici[c.ComponenteID] = c.Codice
		}
	}
	for _, b := range bozze {
		if !nel[b.AllegatoID] {
			continue
		}
		voci = append(voci, dipendenzaDaBozza(b.AllegatoID, b.BozzaID, b.Tipo, codici[b.ProdottoID]))
	}
	// D-10: il perimetro di un archivio
	for _, o := range occ {
		if n, ok := nVoci[o.AllegatoID]; ok && n > 0 {
			voci = append(voci, Dipendenza{AllegatoID: o.AllegatoID, Codice: "D-10", Esito: EsitoMostra,
				Testo: fmt.Sprintf("L'archivio ha %d voci non scartate: seguono l'archivio nello stesso gesto.", n), Rif: strconv.Itoa(n)})
		}
	}
	// D-13: i contesti partner che citano l'occorrenza nel loro perimetro
	if s.Contesti != nil {
		for _, c := range s.Contesti(thread) {
			for _, o := range occ {
				if citaOccorrenza(c, o) {
					voci = append(voci, Dipendenza{AllegatoID: o.AllegatoID, Codice: "D-13", Esito: EsitoMostra, Rif: c.ID,
						Testo: "Il contesto partner " + c.ID + " cita questo file: il contesto non cambia, ma il motore non userà più il file."})
				}
			}
		}
	}
	// D-14: l'unica copia, sui byte che restano dopo il gesto
	uniche, err := s.unicheCopie(ctx, q, thread, occ, ids, nel)
	if err != nil {
		return ElencoDipendenze{}, err
	}
	for _, o := range occ {
		if uniche[o.AllegatoID] {
			voci = append(voci, Dipendenza{AllegatoID: o.AllegatoID, Codice: "D-14", Esito: EsitoRisoluzione,
				Risoluzione: RisoluzioneUnicaCopia,
				Testo:       "È l'unica copia: il file era stato caricato a mano. Scartandolo non ne resta nessuna copia: per riaverlo dovrai ricaricarlo."})
		}
	}
	return chiudiElenco(voci)
}

// chiudiElenco ordina l'elenco (allegato_id, codice, identificativo) e ne calcola l'impronta.
func chiudiElenco(voci []Dipendenza) (ElencoDipendenze, error) {
	if voci == nil {
		voci = []Dipendenza{}
	}
	sort.SliceStable(voci, func(i, j int) bool {
		a, b := voci[i], voci[j]
		if a.AllegatoID != b.AllegatoID {
			return a.AllegatoID.String() < b.AllegatoID.String()
		}
		if a.Codice != b.Codice {
			return a.Codice < b.Codice
		}
		return a.Rif < b.Rif
	})
	h, err := improntaDi(voci)
	if err != nil {
		return ElencoDipendenze{}, err
	}
	return ElencoDipendenze{Voci: voci, Impronta: h}, nil
}

// dipendenzaDaRiga traduce una riga di ListDipendenzeFile nella voce con il suo esito e il suo testo (EM §10.3).
func dipendenzaDaRiga(r db.ListDipendenzeFileRow) Dipendenza {
	d := Dipendenza{AllegatoID: r.AllegatoID, Codice: r.Codice, Esito: EsitoMostra, Rif: r.Rif}
	quale := strings.TrimSpace(r.Etichetta)
	switch r.Codice {
	case "D-1":
		d.Esito = EsitoBlocca
		d.Testo = "È un documento confermato: si sostituisce o si revisiona, non si scarta."
	case "D-2":
		d.Testo = "La copia sul NAS di un documento con lo stesso contenuto è in attesa: la copia nello staging si elimina quando è finita."
	case "D-3":
		d.Esito = EsitoBlocca
		if quale == "" {
			quale = "un prodotto"
		}
		d.Testo = "È lo STEP autorizzato di " + quale + ": prima revoca l'autorizzazione, con il motivo."
	case "D-4":
		d.Testo = "Le proposte di struttura lette da questo file restano come storia."
	case "D-5":
		d.Testo = fmt.Sprintf("%d pezzi della BOM vengono da questo file: restano nella BOM.", r.N)
	case "D-7":
		d.Esito = EsitoRisoluzione
		d.Risoluzione = RisoluzioneScollega
		if quale == "" {
			quale = "un componente"
		}
		d.Testo = "È associato a " + quale + ": scartandolo, l'associazione viene tolta."
	case "D-7b":
		d.Testo = "Esce dai documenti da confermare."
	case "D-8":
		switch r.Rif {
		case string(db.TipoJobAnalizzaAllegato):
			d.Testo = "Un'analisi in corso usa questo file: la copia nello staging si elimina quando finisce."
		case string(db.TipoJobEstraiArchivio):
			d.Testo = "L'estrazione in corso viene annullata."
		default:
			d.Testo = "Il download in corso viene annullato."
		}
	case "D-9":
		d.Testo = fmt.Sprintf("Lo stesso contenuto è usato da altri %d file: la copia nello staging resta per loro.", r.N)
	case "D-11":
		d.Testo = "Le note sul PDF restano."
	case "D-12":
		d.Testo = "C'è una segnalazione aperta sul NAS per questo contenuto: la copia nello staging resta finché non è risolta."
	case "D-15":
		d.Testo = "La storia delle conferme lo cita: resta com'è."
	default:
		d.Testo = r.Codice
	}
	if r.N > 0 && d.Rif == "" {
		d.Rif = strconv.Itoa(int(r.N))
	}
	return d
}

// dipendenzaDaBozza: D-6 (blocca) per la fonte, le caselle, le «Non includere», le preassociazioni, gli svincoli e la
// prova di una decisione identita; D-6b (mostra) per un file citato solo dal registro d'importazione (EM §10.3, K-25).
func dipendenzaDaBozza(allegato, bozza uuid.UUID, tipo, prodotto string) Dipendenza {
	if prodotto == "" {
		prodotto = "un prodotto"
	}
	rif := bozza.String() + ":" + tipo
	if tipo == "registro" {
		return Dipendenza{AllegatoID: allegato, Codice: "D-6b", Esito: EsitoMostra, Rif: rif,
			Testo: "La proposta importata nella bozza di " + prodotto + " lo citava: la bozza tiene la proposta di allora, " +
				"compreso il testo letto da questo file, che resta nel registro; un riallineamento non lo userà più."}
	}
	come := map[string]string{
		"fonte":             "fonte",
		"casella":           "origine di nodi della bozza",
		"casella_formato_2": "origine di nodi della bozza formato 2",
		"non_includere":     "casella di una decisione «Non includere»",
		"preassociazione":   "preassociazione a un nodo",
		"svincolo":          "svincolo di un'associazione",
		"prova_identita":    "prova dell'identità adottata per un nodo",
	}[tipo]
	if come == "" {
		come = tipo
	}
	return Dipendenza{AllegatoID: allegato, Codice: "D-6", Esito: EsitoBlocca, Rif: rif,
		Testo: "È usato dalla bozza di " + prodotto + " (" + come + "): cambia la bozza e salvala, poi riprova."}
}

// citaOccorrenza: un'occorrenza del perimetro del contesto coincide con il file se coincide l'allegato, oppure lo
// sha256 E il nome ricevuto (e il contenitore, se scritto).
func citaOccorrenza(c Contesto, o Occorrenza) bool {
	for _, p := range c.Perimetro {
		if p.AllegatoID != "" {
			if strings.EqualFold(p.AllegatoID, o.AllegatoID.String()) {
				return true
			}
			continue
		}
		if p.Sha256 == "" || p.Nome == "" || !strings.EqualFold(p.Sha256, o.Sha()) || p.Nome != o.NomeFile {
			continue
		}
		if p.Contenitore != "" && (!o.ContenitoreID.Valid || !strings.EqualFold(p.Contenitore, o.ContenitoreID.UUID.String())) {
			continue
		}
		return true
	}
	return false
}

// unicheCopie: le occorrenze dell'elenco dopo il cui gesto non resta nessun byte del file raggiungibile (D-14, EM §10.3;
// giro 1, SIC-01, TR-07). Non contano la copia di conservazione dell'occorrenza, quelle che il gesto stesso rilascia (le
// voci del perimetro) e i riferimenti dell'occorrenza. Contano: un file presente indicato da un'altra occorrenza non
// scartata; la copia copiata di un'altra occorrenza; un documento scritto sul NAS con lo stesso sha nella RFQ; un
// archivio attivo con i byte presenti, da cui la voce si riestrae; l'origine posta, che si riscarica. In blocco: tre
// query per tutto l'elenco, non quattro per occorrenza (un archivio di 1.800 voci, PM23 SC-18).
func (s *Servizio) unicheCopie(ctx context.Context, q *db.Queries, thread uuid.UUID, occ []Occorrenza, esclusi []uuid.UUID,
	nel map[uuid.UUID]bool) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	var candidate []Occorrenza
	perSha := map[string]bool{}
	for _, o := range occ {
		if o.Origine == db.OrigineAllegatoOutlook || o.Sha() == "" {
			continue
		}
		candidate = append(candidate, o)
		perSha[o.Sha()] = true
	}
	if len(candidate) == 0 {
		return out, nil
	}
	sha := make([]string, 0, len(perSha))
	for h := range perSha {
		sha = append(sha, h)
	}
	sort.Strings(sha)
	salvi := map[string]bool{} // i contenuti con i byte raggiungibili altrove
	altre, err := q.ListCopieStagingPerSha(ctx, db.ListCopieStagingPerShaParams{Sha: sha, Esclusi: esclusi})
	if err != nil {
		return nil, err
	}
	for _, a := range altre {
		h := strings.ToLower(strings.TrimSpace(a.Sha256))
		if !salvi[h] && !a.Scartata && presenteNelloStaging(s.Staging, a.PathStaging, h) {
			salvi[h] = true
		}
	}
	copiate, err := q.ListShaConservazioniCopiateAltre(ctx, db.ListShaConservazioniCopiateAltreParams{Sha: sha, Esclusi: esclusi})
	if err != nil {
		return nil, err
	}
	scritti, err := q.ListShaDocumentiScritti(ctx, db.ListShaDocumentiScrittiParams{ThreadID: thread, Sha: sha})
	if err != nil {
		return nil, err
	}
	for _, h := range append(copiate, scritti...) {
		salvi[strings.ToLower(strings.TrimSpace(h))] = true
	}
	// gli archivi fuori dal gesto da cui una voce si riestrae
	var contenitori []uuid.UUID
	visto := map[uuid.UUID]bool{}
	for _, o := range candidate {
		if !salvi[o.Sha()] && o.ContenitoreID.Valid && !nel[o.ContenitoreID.UUID] && !visto[o.ContenitoreID.UUID] {
			visto[o.ContenitoreID.UUID] = true
			contenitori = append(contenitori, o.ContenitoreID.UUID)
		}
	}
	riestraibile := map[uuid.UUID]bool{}
	if len(contenitori) > 0 {
		cc, err := leggiOccorrenze(ctx, q, contenitori)
		if err != nil {
			return nil, err
		}
		for _, c := range cc {
			if Attivo(c.StatoCiclo()) && c.PathStaging.Valid && presenteNelloStaging(s.Staging, c.PathStaging.String, c.Sha()) {
				riestraibile[c.AllegatoID] = true
			}
		}
	}
	for _, o := range candidate {
		if salvi[o.Sha()] || (o.ContenitoreID.Valid && riestraibile[o.ContenitoreID.UUID]) {
			continue
		}
		out[o.AllegatoID] = true
	}
	return out, nil
}

// controllaDipendenze applica al gesto l'elenco ricalcolato sotto il lucchetto (EM §10.4 tempo 1 passo 4; §10.5 fase
// A): prima i blocchi (422 DIPENDENZE_ATTIVE), poi le risoluzioni che mancano (422 RISOLUZIONE_RICHIESTA), poi
// l'impronta (409 ANTEPRIMA_SUPERATA, dipendenze_file, con l'elenco nuovo). scarta: la «conferma dell'unica copia»
// vale solo per «Scarta».
func controllaDipendenze(el ElencoDipendenze, improntaVista string, scollega, unicaCopia, scarta bool) *Rifiuto {
	var blocchi []Dipendenza
	for _, d := range el.Voci {
		if d.Esito == EsitoBlocca {
			blocchi = append(blocchi, d)
		}
	}
	if len(blocchi) > 0 {
		return rifiuto(http.StatusUnprocessableEntity, "DIPENDENZE_ATTIVE",
			"Niente è cambiato: il file ha dipendenze che lo fermano. Toglile e riprova.", extra("dipendenze", blocchi))
	}
	var mancano []Dipendenza
	campi := map[string]bool{}
	for _, d := range el.Voci {
		if d.Esito != EsitoRisoluzione {
			continue
		}
		switch {
		case d.Risoluzione == RisoluzioneScollega && !scollega:
			mancano = append(mancano, d)
			campi[RisoluzioneScollega] = true
		case d.Risoluzione == RisoluzioneUnicaCopia && scarta && !unicaCopia:
			mancano = append(mancano, d)
			campi[RisoluzioneUnicaCopia] = true
		}
	}
	if len(mancano) > 0 {
		var cc []string
		for c := range campi {
			cc = append(cc, c)
		}
		sort.Strings(cc)
		return rifiuto(http.StatusUnprocessableEntity, "RISOLUZIONE_RICHIESTA",
			fmt.Sprintf("Niente è cambiato: %d occorrenze chiedono una risoluzione esplicita (%s).", len(mancano), strings.Join(cc, ", ")),
			extra("occorrenze", mancano, "campi", cc))
	}
	if improntaVista != el.Impronta {
		return rifiuto(http.StatusConflict, "ANTEPRIMA_SUPERATA",
			"Le dipendenze del file sono cambiate mentre guardavi: rivedile e riprova.",
			extra("anteprima", "dipendenze_file", "dipendenze", el.Voci, "impronta_dipendenze", el.Impronta))
	}
	return nil
}
