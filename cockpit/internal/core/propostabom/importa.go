package propostabom

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L'adattatore Motore A → registro d'importazione (V1: emendamento di V1 §3.8, §3.8.1, §3.8.2, §6.1; piano della 0023
// §14 passo 3). ImportaGruppo prende l'esito già calcolato di un thread, il prodotto, il gruppo, le regole di lettura del
// thread e le occorrenze non attive, e restituisce il registro d'importazione del gruppo: il contenuto in nomi minuscoli
// (il DTO di importazione), composto con importazione.Componi, che lo porta nella forma canonica del registro e ne calcola
// le impronte con l'elenco chiuso. Una sola implementazione delle impronte, quella della foglia (P-11, P-19 della corsia
// persistenza; decisioni dell'ondata A). Non legge il database, non scrive, non decide; è pura e deterministica: stesso
// ingresso, stessi byte, stesse impronte (A-01). Nessun orario e nessun ID inventato nel contenuto: gli orari dei fatti e
// gli ID del calcolo stanno in «calcolo», fuori da ogni impronta parziale.

// I codici e i motivi dei rifiuti dell'adattatore (emendamento di V1 §6.1, «Errori dell'importazione»). Un rifiuto è
// un *importazione.Errore con il codice: chi chiama lo traduce nella risposta (422). Un errore senza codice è un difetto
// della forma del registro, cioè dell'adattatore, non della richiesta.
const (
	CodiceCorpoNonValido       = "CORPO_NON_VALIDO"
	CodiceFileNonAttivo        = "FILE_NON_ATTIVO"
	MotivoGruppoNonImportabile = "gruppo_non_importabile"
	MotivoErroreValutazione    = "errore_valutazione"
)

// Ingresso: ciò che serve all'adattatore, tutto già calcolato e in memoria sulla stessa fotografia.
//   - Esito, Thread: l'esito di valutazione.Calcola e il thread chiesto (la testata dà versioni, ImprontaIndice e
//     ImprontaFotografia).
//   - Regole: valutazione.RegoleDiLettura sulla stessa fotografia e sulle stesse regole del calcolo: la grammatica del
//     cliente, il contesto applicato con i partner attivi, la copia canonica e il perimetro per occorrenza (EM §3.8.2;
//     RM-13). L'adattatore non li ricalcola.
//   - Prodotto: il Rif del prodotto valutato. Gruppo: la chiave opaca del gruppo (GruppoBOM o ChiaveCandidata), mai
//     scomposta.
//   - NonAttivi: le occorrenze in «Altri file» o scartate, dal ciclo dei file (EM §10.8): non entrano nel registro.
type Ingresso struct {
	Esito     *valutazione.Esito
	Thread    *valutazione.EsitoThread
	Regole    valutazione.RegoleDelThread
	Prodotto  string
	Gruppo    string
	NonAttivi NonAttivi
}

// ImportaGruppo: il registro d'importazione del gruppo del prodotto (EM §3.8), oppure un rifiuto:
//   - CORPO_NON_VALIDO / gruppo_non_importabile: il prodotto non c'è o non ha un componente; il gruppo non è fra i
//     gruppi del prodotto (anche la stessa chiave di un altro prodotto), è «senza» o non ha una struttura;
//   - FILE_NON_ATTIVO: lo STEP del gruppo è in «Altri file» o scartato;
//   - PROPOSTA_NON_DISPONIBILE / errore_valutazione: il thread non è valutato per un errore della valutazione;
//   - PROPOSTA_NON_DISPONIBILE / registro_incompleto: con un contesto attivo manca qualcosa che il §3.8.2 vuole
//     (partner, contesto, copia, ImprontaIndice, contesto_id e impronta_contesto di un file nel perimetro); mai un
//     registro con un campo null;
//   - IMPORTAZIONE_TROPPO_GRANDE: il registro supera il limite della foglia (mai una troncatura).
//
// Un thread senza la grammatica del cliente si importa: struttura senza identità, con la diagnostica
// importazione.senza_grammatica.
func ImportaGruppo(in Ingresso) (importazione.Registro, error) {
	c, err := contenutoDelRegistro(in)
	if err != nil {
		return importazione.Registro{}, err
	}
	return importazione.Componi(c)
}

func rifiuto(codice, motivo, campo, formato string, a ...any) *importazione.Errore {
	return &importazione.Errore{Codice: codice, Motivo: motivo, Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}

func difetto(campo, formato string, a ...any) *importazione.Errore {
	return &importazione.Errore{Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}

// costruttore: lo stato di una composizione del registro.
type costruttore struct {
	in      Ingresso
	et      *valutazione.EsitoThread
	pv      *valutazione.ProdottoValutato
	st      ancoraggio.StrutturaProdotto
	sf      ancoraggio.StrutturaFile
	gruppo  Gruppo
	nodiBOM map[string]*valutazione.NodoBOM     // i nodi raggiunti del gruppo, per Rif
	nodiSF  map[string]ancoraggio.NodoStruttura // i nodi del grafo del file, per Rif
	file    map[uuid.UUID]*valutazione.FileInterpretato
	lettori map[uuid.UUID]*lettore
	attivo  bool

	// citate: le letture citate da una voce, da un riferimento o da una relazione che devono stare nel registro, per
	// allegato e ID; nelVoci: le chiavi delle letture già scritte nelle voci. Una lettura citata e non scritta nelle voci
	// va fra le letture del suo file.
	citate  map[uuid.UUID]map[string]bool
	nelVoci map[string]bool
	// allegati: le occorrenze citate dal contenuto (fonte, file, prove).
	allegati map[uuid.UUID]bool
	// esclusi: gli allegati non attivi da cui veniva un'evidenza lasciata fuori, con il conto.
	esclusi map[uuid.UUID]int
	// candidati: i file attivi con un candidato fra i nodi del gruppo (fileCandidati), calcolati una volta.
	candidati []uuid.UUID
	// diagTestata: le diagnostiche del registro non riconducibili a un nodo.
	diagTestata []evidenze.Diagnostica
}

func contenutoDelRegistro(in Ingresso) (importazione.Contenuto, error) {
	if in.Esito == nil || in.Thread == nil {
		return importazione.Contenuto{}, difetto("ingresso", "l'adattatore vuole l'esito e il thread già calcolati")
	}
	et := in.Thread
	if et.Motivo == valutazione.MotivoThreadErroreValutazione {
		return importazione.Contenuto{}, rifiuto(importazione.CodicePropostaNonDisponibile, MotivoErroreValutazione, "thread",
			"il thread non è valutato per un errore della valutazione: niente da importare")
	}
	b := &costruttore{in: in, et: et, file: map[uuid.UUID]*valutazione.FileInterpretato{}, lettori: map[uuid.UUID]*lettore{},
		citate: map[uuid.UUID]map[string]bool{}, nelVoci: map[string]bool{}, allegati: map[uuid.UUID]bool{}, esclusi: map[uuid.UUID]int{}}
	for i := range et.File {
		b.file[et.File[i].AllegatoID] = &et.File[i]
	}
	if err := b.trovaIlGruppo(); err != nil {
		return importazione.Contenuto{}, err
	}
	if err := b.controllaIlContesto(); err != nil {
		return importazione.Contenuto{}, err
	}
	return b.componi()
}

// trovaIlGruppo: il prodotto, il gruppo fra i gruppi del prodotto (GruppiDelProdotto: la stessa regola della pagina), la
// sua struttura, i nodi raggiunti e il grafo del file.
func (b *costruttore) trovaIlGruppo() error {
	in, et := b.in, b.et
	for i := range et.ProdottiValutati {
		if et.ProdottiValutati[i].Rif == in.Prodotto {
			b.pv = &et.ProdottiValutati[i]
			break
		}
	}
	if b.pv == nil {
		return rifiuto(CodiceCorpoNonValido, MotivoGruppoNonImportabile, "prodotto_id", "il prodotto %s non è fra i prodotti valutati del thread", in.Prodotto)
	}
	if b.pv.ComponenteID == nil {
		return rifiuto(CodiceCorpoNonValido, MotivoGruppoNonImportabile, "prodotto_id", "il prodotto %s non ha un componente della RFQ", in.Prodotto)
	}
	trovato := false
	for _, g := range GruppiDelProdotto(*b.pv, et.Ancoraggi.Strutture) {
		if g.Chiave == in.Gruppo {
			b.gruppo, trovato = g, true
			break
		}
	}
	if !trovato || b.gruppo.Tipo == GruppoSenzaSTEP || b.gruppo.AllegatoID == nil {
		return rifiuto(CodiceCorpoNonValido, MotivoGruppoNonImportabile, "gruppo", "il gruppo non è fra i gruppi con uno STEP del prodotto")
	}
	allegato := *b.gruppo.AllegatoID
	if stato, ok := b.in.NonAttivi[allegato]; ok {
		return rifiuto(CodiceFileNonAttivo, "", "gruppo", "lo STEP del gruppo non è attivo (%s)", stato)
	}
	stato := ancoraggio.StatoStrutturaCandidata
	if b.gruppo.Tipo == GruppoBOM {
		stato = ancoraggio.StatoBOMDiLavoroProposta
	}
	ok := false
	for _, s := range et.Ancoraggi.Strutture {
		if s.Target == b.pv.Rif && s.Stato == stato && s.AllegatoID == allegato && s.Radice == b.gruppo.Radice {
			b.st, ok = s, true
			break
		}
	}
	if !ok {
		return rifiuto(CodiceCorpoNonValido, MotivoGruppoNonImportabile, "gruppo", "il gruppo non ha una struttura del prodotto")
	}
	b.nodiBOM = map[string]*valutazione.NodoBOM{}
	nodi := b.pv.Nodi
	if b.gruppo.Tipo == GruppoCandidata {
		nodi = nil
		for i := range b.pv.StruttureCandidate {
			if sc := &b.pv.StruttureCandidate[i]; sc.AllegatoID == allegato && sc.Radice == b.st.Radice {
				nodi = sc.Nodi
				break
			}
		}
	}
	for i := range nodi {
		if _, ok := b.nodiBOM[nodi[i].Nodo.Rif]; !ok {
			b.nodiBOM[nodi[i].Nodo.Rif] = &nodi[i]
		}
	}
	f := b.file[allegato]
	if f == nil {
		return difetto("gruppo", "l'esito non ha il file dello STEP del gruppo")
	}
	sf, ok := ancoraggio.StrutturaDa(ancoraggio.FileInterpretato{AllegatoID: f.AllegatoID, Documento: f.Documento,
		Interpretazione: f.Interpretazione, Disponibilita: f.Disponibilita, Disegno: f.Disegno})
	if !ok {
		return difetto("gruppo", "il grafo del file dello STEP del gruppo non si ricava")
	}
	b.sf = sf
	b.nodiSF = make(map[string]ancoraggio.NodoStruttura, len(sf.Nodi))
	for _, n := range sf.Nodi {
		b.nodiSF[n.Rif] = n
	}
	return nil
}

// controllaIlContesto: con un contesto attivo il §3.8.2 vuole il contesto (id e impronta), la copia canonica e
// l'impronta dell'indice; i partner possono essere nessuno. Senza, 422 registro_incompleto, niente scritto.
func (b *costruttore) controllaIlContesto() error {
	c := b.in.Regole.Contesto
	b.attivo = c.Attivo()
	if !b.attivo {
		return nil
	}
	switch {
	case c.ID == "" || c.Impronta == "":
		return rifiuto(importazione.CodicePropostaNonDisponibile, importazione.MotivoRegistroIncompleto, "regole.contesto",
			"il contesto attivo non ha l'id o l'impronta")
	case c.CopiaCanonica() == nil:
		return rifiuto(importazione.CodicePropostaNonDisponibile, importazione.MotivoRegistroIncompleto, "contesto",
			"il motore non espone la copia del contesto risolto")
	case b.in.Esito.ImprontaIndice == "":
		return rifiuto(importazione.CodicePropostaNonDisponibile, importazione.MotivoRegistroIncompleto, "calcolo.impronta_indice",
			"il motore non espone l'impronta dell'indice delle regole")
	}
	return nil
}

// lettoreDi: il lettore delle letture di un allegato del thread; nil se l'esito non ha il file.
func (b *costruttore) lettoreDi(a uuid.UUID) *lettore {
	if x, ok := b.lettori[a]; ok {
		return x
	}
	f := b.file[a]
	if f == nil {
		return nil
	}
	x := nuovoLettore(a, f.Documento, b.et.ClienteID, f.Interpretazione.HashSnapshot)
	b.lettori[a] = x
	return x
}

// attivo: l'occorrenza è attiva (non sta fra le non attive).
func (b *costruttore) attiva(a uuid.UUID) bool {
	_, no := b.in.NonAttivi[a]
	return !no
}

// escludi conta un'evidenza lasciata fuori perché viene da un'occorrenza non attiva.
func (b *costruttore) escludi(a uuid.UUID) { b.esclusi[a]++ }

// cita segna una lettura di un allegato che deve stare nel registro.
func (b *costruttore) cita(a uuid.UUID, id string) {
	if b.citate[a] == nil {
		b.citate[a] = map[string]bool{}
	}
	b.citate[a][id] = true
	b.allegati[a] = true
}

// componi: il contenuto del registro.
func (b *costruttore) componi() (importazione.Contenuto, error) {
	st := b.st
	c := importazione.Contenuto{
		FormatoRegistro: importazione.FormatoRegistro, VersioneImportazione: importazione.VersioneImportazione1,
		ThreadID: b.et.ThreadID.String(), ProdottoID: b.pv.ComponenteID.String(),
	}
	b.allegati[st.AllegatoID] = true
	f := b.file[st.AllegatoID]
	c.Gruppo = importazione.Gruppo{Chiave: b.in.Gruppo, Tipo: raw(b.gruppo.Tipo), AllegatoID: st.AllegatoID.String(), Sha256: st.Sha256,
		Radice: st.Radice, NomeFile: raw(f.Documento.Fonte.Provenienza.NomeRicevuto), StatoStruttura: raw(string(st.Stato)),
		RadiceDelFile: raw(st.RadiceDelFile), Compatibilita: raw(string(st.Compatibilita)), Origine: raw(st.Origine),
		GrafoCompleto: raw(st.GrafoCompleto), MotivoGrafo: raw(st.MotivoGrafo)}
	pr := f.Documento.Fonte.Provenienza
	c.Proposta.Fonte = importazione.FonteProposta{AllegatoID: st.AllegatoID.String(), Sha256: st.Sha256, NomeRicevuto: raw(pr.NomeRicevuto),
		ContenitoreID: raw(f.Documento.Fonte.ContenitoreID), PercorsoRicevuto: raw(pr.PercorsoRicevuto)}

	voci, fuori, err := b.voci()
	if err != nil {
		return importazione.Contenuto{}, err
	}
	c.Proposta.Nodi, c.Proposta.FuoriPerDecisione = voci, fuori
	c.Proposta.Archi = b.archi()
	relazioni := b.relazioni()
	file, err := b.fileDelRegistro()
	if err != nil {
		return importazione.Contenuto{}, err
	}
	c.Proposta.File = file
	c.Proposta.Relazioni = relazioni
	b.diagnostiche(c.Proposta.Nodi)
	b.diagnosticheDellAdattatore(c)
	c.Proposta.Diagnostiche = rawDiagnostiche(b.diagTestata)

	citati := make([]string, 0, len(b.allegati))
	for a := range b.allegati {
		citati = append(citati, a.String())
	}
	sort.Strings(citati)
	c.AllegatiCitati = citati
	c.Regole = b.regole()
	c.Calcolo = b.calcolo(file)
	if b.attivo {
		c.Contesto = raw(b.in.Regole.Contesto.CopiaCanonica())
	}
	return c, nil
}

// regole: versioni, grammatica del cliente, partner attivi e contesto (EM §3.8, «regole»; impronta_regole).
func (b *costruttore) regole() importazione.Regole {
	e := b.in.Esito
	r := importazione.Regole{Versioni: importazione.Versioni{
		Ancoraggio: b.et.Ancoraggi.VersioneServizio, Valutazione: raw(e.VersioneValutazione), Composizione: raw(e.VersioneComposizione),
		Importazione: importazione.VersioneImportazione1, Canonico: jsoncanonico.Versione, Limiti: raw(e.VersioneLimiti),
	}}
	if in := b.file[b.st.AllegatoID].Interpretazione; in.ID != "" {
		r.Versioni.Algoritmo, r.Versioni.Router, r.Versioni.Risultato = raw(in.VersioneAlgoritmo), raw(in.VersioneRouter), raw(in.VersioneRisultato)
	}
	if h := b.in.Regole.HashGrammatica; h != "" {
		r.GrammaticaCliente = &importazione.GrammaticaCliente{ClienteID: b.et.ClienteID.String(), Hash: h}
	}
	if b.attivo {
		c := b.in.Regole.Contesto
		partner := []importazione.PartnerAttivo{}
		for _, p := range c.PartnerAttivi() {
			partner = append(partner, importazione.PartnerAttivo{Emittente: "partner:" + p.Emittente, Hash: p.Hash})
		}
		r.Partner = &partner
		r.Contesto = &importazione.ContestoRegole{ID: raw(c.ID), Impronta: c.Impronta}
	}
	return r
}

// calcolo: solo per l'audit, fuori da ogni impronta parziale (EM §3.8, «calcolo»): l'impronta dell'indice e quella
// della fotografia, le impronte dell'ancoraggio e, per file, il documento, l'interpretazione e i fatti con il momento
// del calcolo. I fatti di una voce sono quelli del file della sua occorrenza (origine.allegato_id): non si ripetono per
// voce.
func (b *costruttore) calcolo(file []importazione.FileRegistro) json.RawMessage {
	e, an := b.in.Esito, b.et.Ancoraggi
	var perFile []any
	for _, fr := range file {
		f := b.file[uuid.MustParse(fr.AllegatoID)]
		in := f.Interpretazione
		perFile = append(perFile, map[string]any{
			"allegato_id": fr.AllegatoID, "bundle_id": f.Documento.BundleID,
			"interpretazione": map[string]any{"id": in.ID, "bundle_id": in.BundleID, "impronta": in.Impronta, "impronta_uso": in.ImprontaUso,
				"impronta_limiti": in.ImprontaLimiti},
			"fatti": json.RawMessage(raw(f.Documento.Fonte.RiferimentoFatti)),
		})
	}
	return raw(map[string]any{
		"impronta_indice": e.ImprontaIndice, "impronta_fotografia": e.ImprontaFotografia,
		"ancoraggio": map[string]any{"hash_ingresso": an.HashIngresso, "hash_target": an.HashTarget, "impronta": an.Impronta},
		"file":       perFile,
	})
}

// raw: il JSON di un valore (la forma canonica la fa Componi); nil per un valore che non si codifica, che non capita
// con i tipi di questo file.
func raw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
