// Package valutazione è il percorso puro comune del banco e dell'anteprima (piano A, par.3.3.8, 6.4.6; R42 B): dalla
// fotografia già chiusa produce il «nuovo» del motore A e, per ogni prodotto target della RFQ, i suoi assi (R79).
// Non confronta e non vede mai l'atteso: non importa confronto né una libreria YAML (R2). È puro: nessun DB, file,
// orologio o rete; la fotografia la legge il caricatore, prima e altrove.
//
// In B1 (commit P7a) ci sono gli ingressi del file dei casi (LeggiIngressi), la richiesta del thread
// (RichiestaDelThread), i prodotti target con l'identità di R70 A e R75 A (TargetConfermato) e la fonte
// strutturale di R65 A e R76 b A (FonteProdotto), con il punto d'ingresso ValutaProdotti. In B5 (commit P7b) il
// collegamento con ancoraggio (le strutture, gli ancoraggi dei file, la catena del codice e la riconciliazione), la
// verifica della BOM, nomenclatura e gerarchia (VerificaDellaBOM, R80), e i conflitti di nomenclatura e di gerarchia
// come pezzi (Conflitto); i 2D dei componenti (validità, cartiglio, revisione del documento, primario e relazioni:
// ValiditaDisegno, ValutaDisegno, Primario); la classificazione dei componenti (Classifica) e la completezza
// documentale, l'asse 6 (Completezza). In B6 (commit P7c, fase V1) il punto d'ingresso Calcola con l'esito (Esito,
// EsitoThread, EsitoFuoriRFQ e la sua impronta), il vecchio di ogni file (LeggiCodiceRegistrato, LetturaRegistrata) e i
// record piatti per confronto (FileConfrontabile e i suoi gemelli), con i tipi degli assi 5 e 7, dei nodi e del
// fascicolo; in B6, fase V2, le associazioni dei file con il perimetro, la pertinenza per evidenza e il contesto del
// messaggio, lo scarto, gli orfani, «da smistare», i conflitti dell'asse smistamento e lo smistamento di ogni prodotto,
// l'asse 5 (Smistamento, R81); in B6, fase V3, i nodi della BOM di lavoro (NodoBOM), l'impronta del prodotto
// (ImprontaProdotto), il composto e lo stato del prodotto, l'asse 7 (StatoDelProdotto), e il fascicolo (Fascicolo).
package valutazione

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// ValutazioneProdotti: la parte di B1 e di B5 della valutazione di un thread. La richiesta (il gesto 1 per messaggio),
// i prodotti candidati della mail (sempre da confermare: R60 A), i prodotti target con l'identità, la fonte, lo stato
// della struttura e la verifica della BOM; gli ancoraggi del thread (EsitoAncoraggi: le strutture con i nodi, gli archi
// e la catena del codice, gli ancoraggi dei file, la riconciliazione, con le loro diagnostiche); i conflitti di
// nomenclatura e di gerarchia, come pezzi per prodotto (B6 li compone); i 2D di ogni componente attivo, con il
// primario e le relazioni da confermare (B5, fase 2: la completezza ne fa le voci); la classificazione di ogni
// componente attivo (B5, fase 3: B6 la porta nei nodi della BOM); da B6, fase V2, le associazioni dei file (una per
// allegato), i file da smistare e i conflitti dell'asse smistamento, come pezzi per prodotto (Calcola li compone con gli
// altri); le diagnostiche di valutazione in ordine di (codice, percorso, riferimenti). B6 la porta dentro l'esito del
// thread. Da V3 i prodotti valutati hanno anche i nodi, l'impronta, il composto, lo stato e i motivi.
type ValutazioneProdotti struct {
	ThreadID         uuid.UUID                    `json:"thread_id"`
	Richiesta        ancoraggio.RichiestaValutata `json:"richiesta"`
	Prodotti         ancoraggio.EsitoProdotti     `json:"prodotti"`
	ProdottiValutati []ProdottoValutato           `json:"prodotti_valutati,omitempty"`
	Ancoraggi        ancoraggio.EsitoAncoraggi    `json:"ancoraggi"`
	Conflitti        []Conflitto                  `json:"conflitti,omitempty"`
	Disegni          []DisegniDelComponente       `json:"disegni,omitempty"`
	Classificazioni  []ComponenteClassificato     `json:"classificazioni,omitempty"`
	// B6, V2: lo smistamento del thread (associazioni.go), con lo smistamento di ogni prodotto nei ProdottiValutati.
	Associazioni         []AssociazioneFile     `json:"associazioni,omitempty"`
	DaSmistare           []FileDaSmistare       `json:"da_smistare,omitempty"`
	ConflittiSmistamento []Conflitto            `json:"conflitti_smistamento,omitempty"`
	Diagnostiche         []evidenze.Diagnostica `json:"diagnostiche,omitempty"`
}

// ValutaProdotti: il percorso di B1 e di B5 per un thread della fotografia, quello che Calcola (B6) usa per ogni thread
// (6.4.6, passi 3-6, 6a, 7, 7a, 8, 9, 9a, 9b e 10b, nella parte che riguarda i messaggi, la richiesta, i target, la
// fonte, le strutture, gli ancoraggi, la nomenclatura e la gerarchia, i 2D e la completezza documentale) e, da B6, lo
// smistamento (fase V2), i nodi, l'impronta e lo stato di ogni prodotto (fase V3).
//  1. I documenti dei messaggi (estrazione.DaMessaggio).
//  2. RichiestaDelThread, con il caso del thread se il file dei casi ne ha uno.
//  3. L'interpretazione di ogni messaggio con il suo uso, poi ancoraggio.ProponiProdotti: i candidati della mail.
//  4. I prodotti target (targetDelThread: R70 A, R75 A, T-E1-24).
//  5. La fonte di ogni target (contesto.fonte: R65 A, R76 b A, T-B0-07, T-B0-08, T-E1-09, T-12).
//  6. B5: gli ancoraggi del thread (ancoraggiDelThread: i file, i target e il contesto convertiti dalla fotografia, poi
//     ancoraggio.ProponiAncoraggi, una volta per thread): le strutture e la BOM di lavoro, la catena del codice, gli
//     ancoraggi dei file e la pre-associazione, la riconciliazione (R59 A, R76 A, R85, R86, R87).
//  7. B5: per ogni target lo stato della struttura (StatoStrutturaDelTarget), i conflitti di nomenclatura e di
//     gerarchia, e la verifica della BOM (VerificaDellaBOM con i gesti dell'adattatore di «Conferma l'albero»: R80,
//     R61 A, T-B0-07, T-B0-24, K-02 con la lettura A), con bom.fonte_non_registrata quando il gesto legacy c'è.
//  8. B5, fase 2: i 2D di ogni componente attivo (disegniDelThread: R82, R83, R84, R104; 6.4.6 passo 10b, nella parte
//     dei 2D): la validità, il cartiglio, l'anteprima, la revisione del documento, il primario e le relazioni.
//  9. B5, fase 3: per ogni target la completezza documentale, l'asse 6 (completezzaDelProdotto, poi Completezza: R72 D,
//     R82, R99 A, R102 A, R103 C, K-01 con la lettura A), con documenti.deroga_non_sostituisce_2d; la classificazione di
//     ogni componente attivo (Classifica: T-E1-18, T-E1R-01). Nessuna ConfermaCategoria: in A1c nessun adattatore
//     (LD-19).
//  10. B6, fase V2: lo smistamento del thread (smistamentoThread: le associazioni di ogni allegato con il perimetro, lo
//     scarto e lo stato terminale; la pertinenza per evidenza con il contesto del messaggio; gli orfani; «da smistare»;
//     i conflitti dell'asse smistamento) e lo smistamento di ogni prodotto (Smistamento, senza percorsi di revisione:
//     LD-18); dopo le risposte del 07/10, il contesto con la provenienza per ogni file che conta e
//     valutazione.contesto_discorde (R106 B e R107, precisate).
//  11. B6, fase V3: per ogni prodotto i nodi della BOM di lavoro (nodiDellaBOM: T-B6-11, R91, T-E1-19), l'impronta dei
//     dati decisi (ImprontaProdotto su datiDecisiDelProdotto: R90, T-B0-29; vuota con T-12) e lo stato
//     (StatoDelProdotto: R79, T-E1-01, T-E1-14), con le condizioni nuove della BOM (condizioniNuoveDellaBOM, con R111 A,
//     precisata dall'utente il 07/10). Senza grammatica lo smistamento non si calcola, quindi il prodotto è non_pronto
//     (T-B6-09, T-B6-61).
//
// m è il motore della grammatica del cliente; nil = nessuna grammatica A: niente interpretazioni né candidati, i
// target confermati senza la base letta, e la fonte senza il confronto delle radici (non_determinabile dove servirebbe);
// gli ancoraggi hanno le strutture senza identità.
// caso è il caso del file dei casi per questo thread (Ingressi.CasoDelThread), o nil.
//
// È pura e deterministica: l'ordine degli elenchi della fotografia non conta, e la fotografia di chi chiama non
// cambia. L'errore è di contratto o di un adattatore: un caso di un altro thread o di un altro cliente, un messaggio
// che l'adattatore non legge, un uso dei segmenti che non torna con il documento (un segmento del caso che non c'è),
// un errore di ProponiProdotti o di ProponiAncoraggi.
func ValutaProdotti(f fotorfq.Fotografia, t fotorfq.Thread, m *motorea.Motore, caso *IngressoCaso) (ValutazioneProdotti, error) {
	v, _, err := valutaThread(f, t, m, caso)
	return v, err
}

// valutaThread: ValutaProdotti con il collegamento del thread (i file letti e interpretati una volta, ancoraggi.go), che
// Calcola riusa per i record dei file e per i record piatti; nil con un errore che arriva prima degli ancoraggi.
func valutaThread(f fotorfq.Fotografia, t fotorfq.Thread, m *motorea.Motore, caso *IngressoCaso) (ValutazioneProdotti, *collegamento, error) {
	if caso != nil && (caso.ThreadID == nil || *caso.ThreadID != t.ID || caso.ClienteID != t.ClienteID) {
		return ValutazioneProdotti{}, nil, fmt.Errorf("valutazione: il caso %q non è del thread %s e del suo cliente", caso.ID, t.ID)
	}
	out := ValutazioneProdotti{ThreadID: t.ID}

	messaggi := messaggiInOrdine(t)
	docs := make(map[uuid.UUID]evidenze.DocumentoEvidenze, len(messaggi))
	for _, msg := range messaggi {
		d, err := estrazione.DaMessaggio(msg)
		if err != nil {
			return ValutazioneProdotti{}, nil, fmt.Errorf("valutazione: thread %s: %w", t.ID, err)
		}
		docs[msg.ID] = d
	}
	richiesta, usi := RichiestaDelThread(t, caso, docs)
	out.Richiesta = richiesta

	var interpretati []ancoraggio.MessaggioInterpretato
	if m != nil {
		for _, msg := range messaggi {
			d := docs[msg.ID]
			uso, ok := usi[d.BundleID]
			if !ok {
				uso = evidenze.UsoSconosciuto(d.BundleID)
			}
			r, err := m.Interpreta(d, uso)
			if err != nil {
				return ValutazioneProdotti{}, nil, fmt.Errorf("valutazione: thread %s, messaggio %s: %w", t.ID, msg.ID, err)
			}
			interpretati = append(interpretati, ancoraggio.MessaggioInterpretato{MessaggioID: msg.ID, Documento: d, Interpretazione: r})
		}
	}
	prodotti, err := ancoraggio.ProponiProdotti(interpretati, richiesta)
	if err != nil {
		return ValutazioneProdotti{}, nil, fmt.Errorf("valutazione: thread %s: %w", t.ID, err)
	}
	out.Prodotti = prodotti

	targets, diag := targetDelThread(t, m, caso, prodotti.Candidati, interpretati)
	c := nuovoContesto(f, t, m)
	for i := range targets {
		fonte, d := c.fonte(targets[i])
		targets[i].pv.Fonte = fonte
		diag = append(diag, d...)
	}

	// B5: il collegamento con ancoraggio, poi gli assi 3 e 4, poi i 2D dei componenti.
	esito, col, err := ancoraggiDelThread(t, m, targets, interpretati)
	if err != nil {
		return ValutazioneProdotti{}, nil, fmt.Errorf("valutazione: thread %s: %w", t.ID, err)
	}
	out.Ancoraggi = esito
	b := nuovaBOMDelThread(t)
	var daVerificare []StrutturaDaVerificare
	var gesti []GestiVerificaBOM
	for _, tg := range targets {
		pv := tg.pv
		pv.Struttura = ancoraggio.StatoStrutturaDelTarget(esito.Strutture, pv.Rif)
		s := b.strutturaDaVerificare(pv, esito.Strutture)
		g := b.gestiDaConfermaLAlbero(pv)
		pv.BOM = VerificaDellaBOM(g, s)
		if g.Legacy && (g.Nomenclatura != nil || g.Gerarchia != nil) {
			diag = append(diag, diagnosticaFonteNonRegistrata(pv))
		}
		out.ProdottiValutati = append(out.ProdottiValutati, pv)
		out.Conflitti = append(out.Conflitti, s.Conflitti...)
		daVerificare = append(daVerificare, s)
		gesti = append(gesti, g)
	}
	var dis *disegniThread
	out.Disegni, dis = disegniDelThread(t, m, col, esito, targets)
	ct := nuovaCompletezzaThread(f, t, m, b, dis, out.Disegni, out.ProdottiValutati)
	for i := range out.ProdottiValutati {
		doc, d := ct.completezzaDelProdotto(out.ProdottiValutati[i], daVerificare[i], esito.Strutture, nil)
		out.ProdottiValutati[i].Documenti = doc
		diag = append(diag, d...)
	}
	out.Classificazioni = ct.classificazioniDelThread(nil)
	// B6, V2: lo smistamento, sui prodotti con gli assi di B1 e B5 (la completezza dà le voci certe).
	sm := nuovoSmistamentoThread(f, t, m, col, esito, targets, out.ProdottiValutati, interpretati, b, dis).calcola()
	out.Associazioni, out.DaSmistare, out.ConflittiSmistamento = sm.associazioni, sm.daSmistare, sm.conflitti
	diag = append(diag, sm.diagnostiche...)
	for i := range out.ProdottiValutati {
		out.ProdottiValutati[i].Smistamento = sm.smistamenti[i]
	}
	// B6, V3: i nodi della BOM di lavoro, l'impronta dei dati decisi e lo stato di ogni prodotto, sui prodotti con tutti gli
	// assi. L'impronta non si calcola con T-12 (resta vuota); un suo errore non capita con questi tipi (nessun decimale,
	// nessuna mappa), e con un errore resterebbe vuota: un'impronta non si inventa.
	nt := nuoviNodiThread(t, b, dis, col, out.Disegni, out.Classificazioni, sm.associazioni, sm.conflitti, out.ProdottiValutati, esito.File)
	senzaImpronta := improntaNonDeterminabile(f.Sezioni)
	for i := range out.ProdottiValutati {
		pv := &out.ProdottiValutati[i]
		pv.Nodi = nt.nodiDellaBOM(*pv, esito.Strutture)
		if !senzaImpronta {
			pv.Impronta, _ = ImprontaProdotto(datiDecisiDelProdotto(*pv, t, b))
		}
		pv.Verificato, pv.Stato, pv.Motivi = StatoDelProdotto(*pv, condizioniNuoveDellaBOM(*pv, gesti[i], daVerificare[i], esito.Strutture, t))
	}
	sort.SliceStable(diag, func(i, j int) bool { return chiaveDiagnostica(diag[i]) < chiaveDiagnostica(diag[j]) })
	out.Diagnostiche = diag
	return out, col, nil
}

// chiaveDiagnostica: l'ordine canonico delle diagnostiche, (codice, percorso, riferimenti) e poi il messaggio, come
// in motorea e in ancoraggio.
func chiaveDiagnostica(d evidenze.Diagnostica) string {
	return d.Codice + "\x00" + d.Percorso + "\x00" + strings.Join(d.Rif, "\x01") + "\x00" + d.Messaggio
}
