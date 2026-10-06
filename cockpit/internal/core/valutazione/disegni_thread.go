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

// L'adattatore dei 2D (B5, fase 2; contratto §1.6; T-B0-22, T-B0-31, T-E1R-05, T-E1R-07): dalla fotografia e dagli
// ancoraggi del thread, per ogni componente attivo i suoi 2D come ingressi astratti delle regole (DisegnoDaValutare,
// ComponenteDaConfrontare), poi ValutaDisegno e Primario. Qui si sceglie solo che cosa della fotografia entra; le regole
// sono in disegni.go e revisione2d.go.

// DisegniDelComponente: i 2D di un componente attivo del thread, con il primario e le relazioni da confermare (tipo
// della fase 2, dubbio T-B5-30): la completezza documentale (fase 3) ne fa VoceFabbisogno.Disegni, B6 NodoBOM.Disegni.
type DisegniDelComponente struct {
	ComponenteID uuid.UUID       `json:"componente_id"`
	Gruppo       GruppoDisegni2D `json:"gruppo"`
}

// I valori del DB e degli adattatori che i 2D guardano, ripetuti qui perché i pacchetti che li dichiarano non li
// esportano: il selettore del nome del file e il campo del codice del cartiglio (router-2, righe 7 e 8), il campo della
// revisione del cartiglio (riga 13).
const (
	selettoreNomeFile       = "nome_file"
	campoCartiglioCodice    = "codice"
	campoCartiglioRevisione = "revisione"
)

// voceDisegno: un 2D di un componente prima della regola: da un documento confermato (provenienza confermato), da una
// proposta aperta con «assegna» (manuale) o da un candidato del motore A sul nodo del componente (proposto).
type voceDisegno struct {
	provenienza ancoraggio.OrigineDato
	documento   *fotorfq.DocumentoConfermato
	allegato    *fotorfq.Allegato
}

// nodoDellEntita: un nodo di una struttura di ancoraggio, per lo STEP dell'entità: il Rif, la revisione interpretata
// dell'identità (IdentitaNodo.Revisione, mai rev_grezza: T-E1R-05), il grezzo dell'id con la sua provenienza.
type nodoDellEntita struct {
	rif       string
	revisione *string
	grezzo    ancoraggio.ValoreGrezzo
	allegato  uuid.UUID
}

// disegniThread: ciò che serve ai 2D di un thread, calcolato una volta. Le mappe servono solo a cercare; ordineFile
// sono gli allegati degli ancoraggi, nell'ordine di ancoraggio (per scorrere).
type disegniThread struct {
	t          fotorfq.Thread
	m          *motorea.Motore
	col        *collegamento
	file       map[uuid.UUID]*ancoraggio.FileInterpretato
	ancoraggi  map[uuid.UUID]*ancoraggio.AncoraggioFile
	ordineFile []uuid.UUID
	nodi       map[uuid.UUID][]nodoDellEntita // per componente, in ordine di Rif, senza doppioni
	nodoPerRif map[string]nodoDellEntita
	posizioni  map[string]uuid.UUID      // la posizione di un candidato → il componente del nodo
	decisi     map[uuid.UUID][]uuid.UUID // per allegato: i componenti delle associazioni decise
	formazioni map[string][]string       // per nodo: le formazioni STEP, per la provenienza di documento.rev
}

// disegniDelThread: i 2D di ogni componente attivo del thread (contratto §1.6, «Gruppo dei 2D di un componente»), in
// ordine di componente; un componente senza 2D non c'è. Per ogni componente:
//   - i documenti disegno_2d confermati sul componente, correnti e no (confermato);
//   - le proposte aperte con «assegna» sul componente di un file che è un 2D (manuale);
//   - i candidati del motore A: i 2D con un candidato che ha una posizione su un nodo del componente (la decisione per
//     UUID del nodo, o la radice che rappresenta il prodotto, per il componente del target: T-B5-95, i candidati vengono
//     dagli ancoraggi dei file); un 2D può stare nei gruppi di più componenti (T-B5-96).
//
// Un contenuto solo per gruppo: lo stesso sha256 da più vie dà una voce sola, la più forte (documento corrente, poi
// «assegna», poi candidato, poi documento non corrente: dubbio T-B5-33). Poi ValutaDisegno per ognuno, contro il
// componente, e Primario. Nessuna decisione sui documenti: in A1c non c'è un adattatore (LD-27), quindi nessun
// conflitto identita_documento sui dati veri. Torna anche ciò che serve ai 2D del thread, per la completezza (fase 3: i
// candidati dei nodi senza decisione e quelli da_determinare).
func disegniDelThread(t fotorfq.Thread, m *motorea.Motore, col *collegamento, esito ancoraggio.EsitoAncoraggi, targets []target) ([]DisegniDelComponente, *disegniThread) {
	x := nuoviDisegniThread(t, m, col, esito, targets)
	componenti := append([]fotorfq.Componente(nil), t.Componenti...)
	sort.SliceStable(componenti, func(i, j int) bool { return componenti[i].ID.String() < componenti[j].ID.String() })
	attivi := map[uuid.UUID]*fotorfq.Componente{}
	for i := range componenti {
		if componenti[i].ArchiviatoIl == nil {
			attivi[componenti[i].ID] = &componenti[i]
		}
	}

	voci := map[uuid.UUID][]voceDisegno{}
	docs := append([]fotorfq.DocumentoConfermato(nil), t.Documenti...)
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].ID.String() < docs[j].ID.String() })
	for i := range docs {
		d := &docs[i]
		if d.Tipo == tipoDocumentoDisegno2D && d.ComponenteID != nil && attivi[*d.ComponenteID] != nil {
			voci[*d.ComponenteID] = append(voci[*d.ComponenteID], voceDisegno{provenienza: ancoraggio.OrigineConfermato, documento: d})
		}
	}
	proposte := append([]fotorfq.PropostaAttuale(nil), t.Proposte...)
	sort.SliceStable(proposte, func(i, j int) bool { return proposte[i].ID.String() < proposte[j].ID.String() })
	for _, p := range proposte {
		if p.Stato == statoPropostaAperta && p.ComponenteID != nil && attivi[*p.ComponenteID] != nil && x.file[p.AllegatoID] != nil && x.file[p.AllegatoID].Disegno {
			voci[*p.ComponenteID] = append(voci[*p.ComponenteID], voceDisegno{provenienza: ancoraggio.OrigineManuale, allegato: col.allegati[p.AllegatoID]})
		}
	}
	for _, a := range esito.File {
		if f := x.file[a.AllegatoID]; f == nil || !f.Disegno {
			continue
		}
		visti := map[uuid.UUID]bool{}
		for _, c := range a.Candidati {
			for _, p := range c.Posizioni {
				k, ok := x.posizioni[chiavePosizione(p.Target, p.AllegatoID, p.Radice, p.Nodo)]
				if !ok || attivi[k] == nil || visti[k] {
					continue
				}
				visti[k] = true
				voci[k] = append(voci[k], voceDisegno{provenienza: ancoraggio.OrigineProposto, allegato: col.allegati[a.AllegatoID]})
			}
		}
	}

	var out []DisegniDelComponente
	for _, k := range componenti {
		if len(voci[k.ID]) == 0 {
			continue
		}
		c := x.componenteDaConfrontare(attivi[k.ID])
		var disegni []Disegno2D
		for _, v := range unaPerContenuto(voci[k.ID]) {
			d, _ := ValutaDisegno(x.daValutare(v, k.ID), c, nil)
			disegni = append(disegni, d)
		}
		out = append(out, DisegniDelComponente{ComponenteID: k.ID, Gruppo: Primario(disegni)})
	}
	return out, x
}

func nuoviDisegniThread(t fotorfq.Thread, m *motorea.Motore, col *collegamento, esito ancoraggio.EsitoAncoraggi, targets []target) *disegniThread {
	x := &disegniThread{t: t, m: m, col: col, file: map[uuid.UUID]*ancoraggio.FileInterpretato{}, ancoraggi: map[uuid.UUID]*ancoraggio.AncoraggioFile{},
		nodi: map[uuid.UUID][]nodoDellEntita{}, nodoPerRif: map[string]nodoDellEntita{}, posizioni: map[string]uuid.UUID{},
		decisi: map[uuid.UUID][]uuid.UUID{}, formazioni: map[string][]string{}}
	for i := range col.file {
		x.file[col.file[i].AllegatoID] = &col.file[i]
	}
	for i := range esito.File {
		x.ancoraggi[esito.File[i].AllegatoID] = &esito.File[i]
		x.ordineFile = append(x.ordineFile, esito.File[i].AllegatoID)
	}
	componenteDelTarget := map[string]*uuid.UUID{}
	for _, tg := range targets {
		componenteDelTarget[tg.pv.Rif] = tg.pv.ComponenteID
	}
	// I nodi dei componenti: la decisione per UUID del nodo, o la radice che rappresenta il prodotto (la sua base, o la
	// radice scelta della BOM di lavoro) per il componente del target, come per l'origine dell'associazione di
	// ancoraggio (T-B4-37).
	visti := map[string]bool{}
	for _, s := range esito.Strutture {
		for _, n := range s.Nodi {
			var k *uuid.UUID
			switch {
			case n.Decisione != nil:
				id := n.Decisione.ComponenteID
				k = &id
			case n.Rif == s.Radice && radiceDelProdotto(s):
				k = componenteDelTarget[s.Target]
			}
			if _, ok := x.nodoPerRif[n.Rif]; !ok {
				x.nodoPerRif[n.Rif] = nodoDellEntita{rif: n.Rif, revisione: copiaTesto(n.Codice.Identita.Revisione), grezzo: n.Codice.Grezzo, allegato: n.AllegatoID}
			}
			if k == nil {
				continue
			}
			x.posizioni[chiavePosizione(s.Target, s.AllegatoID, s.Radice, n.Rif)] = *k
			if kk := k.String() + "\x00" + n.Rif; !visti[kk] {
				visti[kk] = true
				x.nodi[*k] = append(x.nodi[*k], x.nodoPerRif[n.Rif])
			}
		}
	}
	for k := range x.nodi {
		sort.SliceStable(x.nodi[k], func(i, j int) bool { return x.nodi[k][i].rif < x.nodi[k][j].rif })
	}
	for _, a := range col.associazioniDecise() {
		x.decisi[a.AllegatoID] = append(x.decisi[a.AllegatoID], a.ComponenteID)
	}
	for _, s := range col.strutture {
		for _, n := range s.Nodi {
			if len(n.Formazioni) > 0 && x.formazioni[n.Rif] == nil {
				x.formazioni[n.Rif] = n.Formazioni
			}
		}
	}
	return x
}

// radiceDelProdotto: la radice della struttura rappresenta il prodotto: la sua base è compatibile con quella del target,
// o è la radice scelta della BOM di lavoro (R76 A). È la regola di ancoraggio per l'origine dell'associazione del 2D del
// prodotto (T-B4-37), ripetuta perché ancoraggio non la esporta.
func radiceDelProdotto(s ancoraggio.StrutturaProdotto) bool {
	switch s.Compatibilita {
	case motorea.CompatibilitaUguale, motorea.CompatibilitaEquivalente, motorea.CompatibilitaParziale:
		return true
	}
	return s.Stato == ancoraggio.StatoBOMDiLavoroProposta
}

// chiavePosizione: la chiave di una posizione di un candidato (target, allegato e radice della struttura, nodo).
func chiavePosizione(target string, allegato uuid.UUID, radice, nodo string) string {
	return target + "\x00" + allegato.String() + "\x00" + radice + "\x00" + nodo
}

// unaPerContenuto: le voci di un componente, una per contenuto (lo sha256; senza, il documento o l'allegato), la più
// forte (rangoVoce): un documento corrente, poi «assegna», poi un candidato, poi un documento non corrente; a parità il
// confermato più di recente, poi l'ID del documento e dell'allegato. In ordine di chiave.
func unaPerContenuto(voci []voceDisegno) []voceDisegno {
	per := map[string]voceDisegno{}
	var chiavi []string
	for _, v := range voci {
		k := chiaveContenuto(v)
		w, ok := per[k]
		if !ok {
			chiavi = append(chiavi, k)
			per[k] = v
			continue
		}
		if piuForte(v, w) {
			per[k] = v
		}
	}
	sort.Strings(chiavi)
	out := make([]voceDisegno, 0, len(chiavi))
	for _, k := range chiavi {
		out = append(out, per[k])
	}
	return out
}

func chiaveContenuto(v voceDisegno) string {
	switch {
	case v.documento != nil && v.documento.Sha256 != "":
		return "sha:" + v.documento.Sha256
	case v.documento != nil:
		return "documento:" + v.documento.ID.String()
	case v.allegato != nil && v.allegato.Sha256 != nil && *v.allegato.Sha256 != "":
		return "sha:" + *v.allegato.Sha256
	case v.allegato != nil:
		return "allegato:" + v.allegato.ID.String()
	}
	return ""
}

// rangoVoce: la forza di una voce per lo stesso contenuto (contratto §1.6, la riga «Da verificare»; R62 b A; dubbio
// T-B5-33 corretto con la revisione della fase 2): il documento corrente chiude la voce; poi «assegna» e il candidato,
// che restano da verificare (associazione_non_confermata); il documento non corrente per ultimo, perché per la voce non
// conta: un'associazione aperta non sparisce dietro un documento sostituito.
func rangoVoce(v voceDisegno) int {
	switch {
	case v.documento != nil && v.documento.SostituitoDa == nil:
		return 3
	case v.documento != nil:
		return 0
	case v.provenienza == ancoraggio.OrigineManuale:
		return 2
	}
	return 1
}

// piuForte: v vale più di w per lo stesso contenuto.
func piuForte(v, w voceDisegno) bool {
	if rv, rw := rangoVoce(v), rangoVoce(w); rv != rw {
		return rv > rw
	}
	if v.documento != nil && w.documento != nil {
		if !v.documento.ConfermatoIl.Equal(w.documento.ConfermatoIl) {
			return v.documento.ConfermatoIl.After(w.documento.ConfermatoIl)
		}
		return v.documento.ID.String() < w.documento.ID.String()
	}
	if v.allegato != nil && w.allegato != nil {
		return v.allegato.ID.String() < w.allegato.ID.String()
	}
	return false
}

// gruppoDelNodo: i 2D candidati di un nodo senza decisione, per FabbisognoPrevisto.Disegni (fase 3): i file 2D con un
// candidato del motore A che ha una posizione sul nodo nelle strutture del prodotto (la stessa raccolta dei candidati di
// un componente, con il nodo al posto del componente), una voce per contenuto, valutati contro l'identità del nodo
// (la sua lettura e la revisione interpretata, mai rev_grezza: T-E1R-05), poi Primario. nil senza candidati.
func (x *disegniThread) gruppoDelNodo(prodotto string, n ancoraggio.NodoProposto) *GruppoDisegni2D {
	var voci []voceDisegno
	for _, id := range x.ordineFile {
		f, a := x.file[id], x.ancoraggi[id]
		if f == nil || !f.Disegno || a == nil {
			continue
		}
		trovato := false
		for _, c := range a.Candidati {
			for _, p := range c.Posizioni {
				trovato = trovato || (p.Target == prodotto && p.Nodo == n.Rif)
			}
		}
		if trovato {
			voci = append(voci, voceDisegno{provenienza: ancoraggio.OrigineProposto, allegato: x.col.allegati[id]})
		}
	}
	if len(voci) == 0 {
		return nil
	}
	nodo := ComponenteDaConfrontare{Confronto: ConfrontatoConNessuno}
	if l := n.Codice.Forma; l != nil {
		v := *l
		v.Base = copiaBase(v.Base)
		nodo.Lettura = &v
	}
	if r := n.Codice.Identita.Revisione; r != nil {
		nodo.Revisione, nodo.FonteRevisione, nodo.Confronto = copiaTesto(r), FonteComponenteStepEntita, ConfrontatoConProposto
	}
	var disegni []Disegno2D
	for _, v := range unaPerContenuto(voci) {
		d, _ := ValutaDisegno(x.daValutare(v, uuid.Nil), nodo, nil)
		disegni = append(disegni, d)
	}
	g := Primario(disegni)
	return &g
}

// candidatiDaDeterminare: gli allegati con il tipo documentale da_determinare (LD-17: la vista non li vede), in un
// formato configurato dei 2D, con un candidato del motore A che ha una posizione su un nodo del componente (la decisione
// per UUID del nodo, o la radice che rappresenta il prodotto), nell'ordine degli ancoraggi: per la voce del 2D sono
// un'associazione da verificare (contratto §1.6, «Da verificare»; fase 3).
func (x *disegniThread) candidatiDaDeterminare(componente uuid.UUID) []uuid.UUID {
	var out []uuid.UUID
	for _, id := range x.ordineFile {
		a, all := x.ancoraggi[id], x.col.allegati[id]
		if a == nil || all == nil || x.col.tipi[id] != tipoDocumentoDaDeterminare || FormatoDi(estensioneDi(all)) == "" {
			continue
		}
		trovato := false
		for _, c := range a.Candidati {
			for _, p := range c.Posizioni {
				if k, ok := x.posizioni[chiavePosizione(p.Target, p.AllegatoID, p.Radice, p.Nodo)]; ok && k == componente {
					trovato = true
				}
			}
		}
		if trovato {
			out = append(out, id)
		}
	}
	return out
}

// ---- un 2D ----

// contenutoLetto: il documento e l'interpretazione del contenuto di un 2D, con l'allegato da cui vengono (nil se il
// contenuto è letto dai fatti del documento); ok falso se l'adattatore non lo legge.
type contenutoLetto struct {
	doc      evidenze.DocumentoEvidenze
	interp   motorea.Interpretazione
	allegato *uuid.UUID
	ok       bool
}

// daValutare: l'ingresso astratto di ValutaDisegno per una voce (DisegnoDaValutare): il file, il contenuto con la
// validità (ValiditaDisegno), il cartiglio, l'anteprima, la revisione registrata con la provenienza (T-E1R-07), le
// fonti d'identità (il cartiglio, lo STEP dell'entità, il nome del file, il documento), la rev_diversa della vista.
func (x *disegniThread) daValutare(v voceDisegno, componente uuid.UUID) DisegnoDaValutare {
	d := Disegno2D{Provenienza: v.provenienza}
	var cont contenutoLetto
	var nome, estensione string
	if doc := v.documento; doc != nil {
		did, il := doc.ID, doc.ConfermatoIl
		d.DocumentoID, d.Sha256, d.Corrente, d.ConfermatoIl = &did, doc.Sha256, doc.SostituitoDa == nil, &il
		nome, estensione = doc.NomeFile, doc.Estensione
		for _, a := range doc.Allegati {
			if x.file[a] != nil {
				id := a
				d.AllegatoID = &id
				break
			}
		}
		cont = x.contenutoDelDocumento(doc)
		if r := strings.TrimSpace(testoDi(doc.Rev)); r != "" {
			d.Registrata, d.RevProvenienza = &r, x.provenienzaRegistrata(*doc.ComponenteID, r)
		}
	} else {
		a := v.allegato
		id := a.ID
		d.AllegatoID, nome, estensione = &id, a.NomeFile, estensioneDi(a)
		if a.Sha256 != nil {
			d.Sha256 = *a.Sha256
		}
		if f := x.file[a.ID]; f != nil {
			cont = contenutoLetto{doc: f.Documento, interp: f.Interpretazione, allegato: &id, ok: true}
		}
	}
	d.NomeFile, d.Estensione, d.Formato = nome, normaEstensione(estensione), FormatoDi(estensione)
	c := contenutoDelDisegno(d.Formato, cont)
	d.Validita, d.MotivoValidita = ValiditaDisegno(c)
	d.Testo = c.Testo
	d.Cartiglio = statoCartiglio(d.Formato, cont)
	d.Anteprima = RiferimentoAnteprima{Sha256: d.Sha256, Formato: d.Formato, DocumentoID: copiaUUID(d.DocumentoID), AllegatoID: copiaUUID(d.AllegatoID),
		Disponibile: anteprimaDisponibile(d.Formato, d.MotivoValidita)}

	in := DisegnoDaValutare{Disegno: d}
	in.Fonti = append(in.Fonti, fonteCartiglio(d.Cartiglio, cont), x.fonteStepEntita(v, componente), fonteNome(x.m, nome))
	if doc := v.documento; doc != nil {
		in.Fonti = append(in.Fonti, x.fonteDocumento(doc, d.Registrata))
		in.DiscordanzaVista = x.revDiversaDellaVista(*doc.ComponenteID, doc.ID)
	}
	return in
}

// contenutoDelDocumento: il contenuto di un documento confermato: quello del primo dei suoi allegati che è un file del
// thread, altrimenti quello di un file del thread con lo stesso sha256 (in ordine di allegato), altrimenti dai fatti del
// documento (T-04), con un record d'allegato che serve solo all'adattatore per leggerli e non esce da qui, come per lo
// STEP di un documento (fonte.go, analisiDocumento).
func (x *disegniThread) contenutoDelDocumento(doc *fotorfq.DocumentoConfermato) contenutoLetto {
	for _, a := range doc.Allegati {
		if f := x.file[a]; f != nil {
			id := a
			return contenutoLetto{doc: f.Documento, interp: f.Interpretazione, allegato: &id, ok: true}
		}
	}
	for i := range x.col.file {
		f := &x.col.file[i]
		if a := x.col.allegati[f.AllegatoID]; doc.Sha256 != "" && a != nil && a.Sha256 != nil && *a.Sha256 == doc.Sha256 {
			id := f.AllegatoID
			return contenutoLetto{doc: f.Documento, interp: f.Interpretazione, allegato: &id, ok: true}
		}
	}
	var fatti *fotorfq.Fatti
	if f, ok := x.t.Fatti[doc.Sha256]; ok && doc.Sha256 != "" {
		fatti = &f
	}
	est, sha := doc.Estensione, doc.Sha256
	d, err := estrazione.DaAllegato(fotorfq.Allegato{ID: doc.ID, NomeFile: doc.NomeFile, Estensione: &est, Sha256: &sha, Natura: naturaFile}, fatti, nil)
	if err != nil {
		return contenutoLetto{}
	}
	interp := motorea.Interpretazione{BundleID: d.BundleID}
	if x.m != nil {
		if interp, err = x.m.Interpreta(d, evidenze.UsoSconosciuto(d.BundleID)); err != nil {
			return contenutoLetto{}
		}
	}
	return contenutoLetto{doc: d, interp: interp, ok: true}
}

// contenutoDelDisegno: l'adattatore dei fatti al ContenutoDisegno (T-B0-22, T-B0-31; T-09: dalle diagnostiche del
// documento, i codici esportati da estrazione), il primo che vale:
//   - un formato non configurato: formato_non_configurato;
//   - TIFF o PNG: nessun fatto di decodifica (LD-01): contenuto_non_verificabile;
//   - un PDF che l'adattatore non legge, o senza fatti (pdf.non_analizzato): contenuto_non_analizzato, e con i fatti in
//     un payload che non si legge contenuto_non_letto;
//   - errore_pdf (pdf.illeggibile): contenuto_non_aperto;
//   - fatti senza testo né errore (pdf.non_letto): contenuto_non_letto;
//   - altrimenti decodificato (testo_pdf, anche una scansione), con il testo se non è pdf.senza_testo.
func contenutoDelDisegno(f Formato2D, c contenutoLetto) ContenutoDisegno {
	switch {
	case f == "":
		return ContenutoDisegno{Motivo: MotivoFabbisognoFormatoNonConfigurato}
	case f != Formato2DPDF:
		return ContenutoDisegno{Formato: f, Motivo: MotivoFabbisognoContenutoNonVerificabile}
	case !c.ok:
		return ContenutoDisegno{Formato: f, Motivo: MotivoFabbisognoContenutoNonLetto}
	case haDiagnostica(c.doc, estrazione.CodicePDFNonAnalizzato):
		return ContenutoDisegno{Formato: f, Motivo: MotivoFabbisognoContenutoNonAnalizzato}
	case haDiagnostica(c.doc, estrazione.CodicePDFIlleggibile):
		return ContenutoDisegno{Formato: f, Motivo: MotivoFabbisognoContenutoNonAperto}
	case haDiagnostica(c.doc, estrazione.CodicePDFNonLetto):
		return ContenutoDisegno{Formato: f, Motivo: MotivoFabbisognoContenutoNonLetto}
	}
	return ContenutoDisegno{Formato: f, Decodificato: true, Testo: !haDiagnostica(c.doc, estrazione.CodicePDFSenzaTesto)}
}

// statoCartiglio: il cartiglio di un 2D (T-E1-21; dubbio T-B5-37, deciso [T] dall'orchestratore): letto per un PDF
// letto con almeno un'unità con il selettore del cartiglio, dal testo nativo o dall'OCR, come le legge ancoraggio per
// la riconciliazione: E1R §5.2 dice «cartiglio leggibile» senza distinguere, e l'OCR è una capacità aggiuntiva (R83,
// LD-04). Così valutazione e ancoraggio non si contraddicono sullo stesso file, e un'evidenza letta non esce senza
// valore. La capacità cartiglio dell'adattatore non conta: dice il testo nativo. Il testo del worker di prima resta
// non_leggibile da solo, perché i suoi campi sono unità testo_pdf, non del cartiglio. non_leggibile per un altro PDF;
// formato_senza_lettura per gli altri formati (LD-04).
func statoCartiglio(f Formato2D, c contenutoLetto) string {
	if f != Formato2DPDF {
		return CartiglioFormatoSenzaLettura
	}
	if c.ok {
		for _, u := range c.doc.Unita {
			if u.Selettore.Contesto == evidenze.ContestoCartiglio {
				return CartiglioLetto
			}
		}
	}
	return CartiglioNonLeggibile
}

// anteprimaDisponibile: l'anteprima di un 2D (LD-03; R83; dubbio T-B5-38 corretto con la revisione della fase 2): oggi
// solo per il PDF, e non per un PDF che il worker non apre (errore_pdf, contenuto_non_aperto): il contenuto non si apre,
// quindi l'anteprima non c'è. Un PDF non analizzato o non letto la ha: A1c non vede i byte, e il web serve ciò che
// comincia con %PDF-.
func anteprimaDisponibile(f Formato2D, motivo MotivoFabbisogno) bool {
	return f == Formato2DPDF && motivo != MotivoFabbisognoContenutoNonAperto
}

// ---- le fonti d'identità ----

// fonteCartiglio: il cartiglio come fonte (E1R §5.2, punto 1). Un cartiglio che non si legge non dà niente, con il
// motivo (cartiglio_non_leggibile, formato_senza_lettura). Altrimenti le letture di cartiglio.codice del 2D (la
// funzione identita_file) danno il codice, se sono una sola identità (identitaDaLetture), e la revisione in linea; la
// revisione del campo a sé (cartiglio.revisione) entra come attributo della grammatica: attribuita, con il valore;
// altrimenti con il solo testo, non interpretabile (C-34). Un cartiglio leggibile senza revisione: un'evidenza senza
// valore, revisione_assente_nel_cartiglio, e la proposta passa allo STEP dell'entità (decisioni dell'orchestratore,
// punto 5; dubbio T-B5-32). La coppia è il testo di cartiglio.codice (T-B4-22).
func fonteCartiglio(stato string, c contenutoLetto) FonteDelDisegno {
	f := FonteDelDisegno{Revisione: EvidenzaRevisione{Fonte: FonteEvidenzaCartiglio}, AllegatoID: copiaUUID(c.allegato)}
	switch stato {
	case CartiglioFormatoSenzaLettura:
		f.Revisione.Motivo = MotivoEvidenzaFormatoSenzaLettura
		return f
	case CartiglioNonLeggibile:
		f.Revisione.Motivo = MotivoEvidenzaCartiglioNonLeggibile
		return f
	}
	unita := make(map[string]evidenze.UnitaEvidenza, len(c.doc.Unita))
	var codici []evidenze.UnitaEvidenza
	for _, u := range c.doc.Unita {
		unita[u.ID] = u
		if u.Selettore.Contesto == evidenze.ContestoCartiglio && u.Selettore.Campo.Valore == campoCartiglioCodice {
			codici = append(codici, u)
		}
	}
	sort.SliceStable(codici, func(i, j int) bool { return codici[i].ID < codici[j].ID })
	if len(codici) > 0 {
		f.Coppia = ancoraggio.EvidenzaDa(FonteEvidenzaCartiglio, codici[0].Testo)
		f.UnitaID, f.Posizione = codici[0].ID, codici[0].Posizione
	}
	var letture []motorea.LetturaForma
	perForma := map[string]string{} // l'unità di ogni lettura, per la provenienza
	for _, l := range c.interp.Letture {
		if s := l.Forma.Selettore; l.Funzione == motorea.FunzIdentitaFile && s.Contesto == evidenze.ContestoCartiglio && s.Campo.Valore == campoCartiglioCodice {
			letture = append(letture, l.Forma)
			perForma[chiaveForma(l.Forma)] = l.UnitaID
		}
	}
	scelta, discordanti, valori, grezzi := identitaDaLetture(letture)
	for _, a := range c.interp.Attributi {
		u, ok := unita[a.UnitaID]
		if a.Tipo != motorea.AttributoRevisione || !ok || u.Selettore.Contesto != evidenze.ContestoCartiglio || u.Selettore.Campo.Valore != campoCartiglioRevisione {
			continue
		}
		if a.Stato == motorea.StatoAttribuito && strings.TrimSpace(a.Normalizzato) != "" {
			valori = append(valori, strings.TrimSpace(a.Normalizzato))
		} else if g := strings.TrimSpace(a.Grezzo); g != "" {
			grezzi = append(grezzi, g)
		}
	}
	if scelta != nil && !discordanti {
		f.Lettura, f.Codice = scelta, scelta.Originale
		if id, ok := perForma[chiaveForma(*scelta)]; ok {
			f.UnitaID, f.Posizione = id, unita[id].Posizione
		}
	}
	f.Revisione = evidenzaDaValori(FonteEvidenzaCartiglio, valori, grezzi, discordanti, MotivoEvidenzaRevisioneAssenteCartiglio)
	return f
}

// fonteNome: il nome del file come fonte (E1R §5.2, punto 3): le letture del nome con la grammatica del cliente, sul
// selettore nome_file (motorea.Riconosci), con la stessa regola delle letture del cartiglio (identitaDaLetture). La
// coppia è il nome com'è (T-B4-22).
func fonteNome(m *motorea.Motore, nome string) FonteDelDisegno {
	f := FonteDelDisegno{Revisione: EvidenzaRevisione{Fonte: FonteEvidenzaNomeFile, Motivo: MotivoEvidenzaNomeNonLetto},
		Coppia: ancoraggio.EvidenzaDa(FonteEvidenzaNomeFile, nome)}
	sel, err := evidenze.LeggiSelettore(selettoreNomeFile)
	if m == nil || err != nil || strings.TrimSpace(nome) == "" {
		return f
	}
	letture, _ := m.Riconosci(sel, nome)
	scelta, discordanti, valori, grezzi := identitaDaLetture(letture)
	if scelta == nil {
		return f
	}
	if !discordanti {
		f.Lettura, f.Codice = scelta, scelta.Originale
	}
	f.Revisione = evidenzaDaValori(FonteEvidenzaNomeFile, valori, grezzi, discordanti, MotivoEvidenzaRevisioneAssenteNome)
	return f
}

// identitaDaLetture: le letture complete (base completa) di una fonte (le letture del cartiglio, del nome):
//   - scelta: la lettura che dà il codice: la prima con una revisione letta, altrimenti la prima, in ordine di
//     intervallo, famiglia e forma; nil senza letture complete;
//   - discordanti: due letture non danno la stessa identità (lo stesso namespace, la stessa base, i marcatori scritti
//     uguali: la regola unica del marcatore, T-B4-30): nessuna si sceglie (T-E1-07);
//   - valori, grezzi: le revisioni lette (la normalizzata) e quelle che non si interpretano (il testo), senza doppioni.
//     Una lettura senza revisione non discorda con una che la porta (come per le identità del file di ancoraggio:
//     «revisioni lette diverse»).
func identitaDaLetture(letture []motorea.LetturaForma) (*motorea.LetturaForma, bool, []string, []string) {
	var complete []motorea.LetturaForma
	for _, l := range letture {
		if l.Base.Completa {
			complete = append(complete, l)
		}
	}
	if len(complete) == 0 {
		return nil, false, nil, nil
	}
	sort.SliceStable(complete, func(i, j int) bool { return chiaveForma(complete[i]) < chiaveForma(complete[j]) })
	discordanti := false
	var valori, grezzi []string
	scelta := -1
	for i, l := range complete {
		for _, k := range complete[i+1:] {
			if l.Namespace != k.Namespace || compatibilitaCodici(&l, &k) != motorea.CompatibilitaUguale {
				discordanti = true
			}
		}
		if r := l.Revisione; r != nil {
			if r.Stato == motorea.StatoRevisioneLetta && strings.TrimSpace(r.Normalizzata) != "" {
				valori = append(valori, strings.TrimSpace(r.Normalizzata))
				if scelta < 0 {
					scelta = i
				}
			} else if g := strings.TrimSpace(r.Originale); g != "" {
				grezzi = append(grezzi, g)
			}
		}
	}
	if scelta < 0 {
		scelta = 0
	}
	s := complete[scelta]
	s.Base = copiaBase(s.Base)
	s.Revisione = copiaRevisione(s.Revisione)
	return &s, discordanti, valori, grezzi
}

// chiaveForma: l'ordine fisso delle letture di una fonte (intervallo, famiglia, forma, testo), con gli intervalli a
// dieci cifre perché l'ordine dei testi sia quello dei numeri.
func chiaveForma(l motorea.LetturaForma) string {
	return fmt.Sprintf("%010d\x00%010d\x00%s\x00%s\x00%s", l.Intervallo.Inizio, l.Intervallo.Fine, l.Famiglia, l.Forma, l.Originale)
}

// evidenzaDaValori: l'evidenza di revisione di una fonte dalle revisioni trovate (dubbio T-B5-32), il primo che vale:
// letture discordanti (nessun valore); nessuna revisione (il motivo dell'assenza); una revisione che non si interpreta
// (il testo, se è l'unica cosa trovata); più revisioni lette diverse (letture_discordanti); altrimenti la revisione,
// interpretabile. Un testo non interpretato uguale a una revisione letta è la stessa revisione, e non la rende dubbia;
// il confronto è esatto dopo gli spazi ai bordi (stessaRevisione, come in ancoraggio: decisione R-14 sulla fase 2,
// allineata qui con T-B5-46): «a» accanto a «A» non è la stessa revisione, e dà revisione_non_interpretabile senza
// valore.
func evidenzaDaValori(fonte string, valori, grezzi []string, discordanti bool, motivoAssente string) EvidenzaRevisione {
	valori = ordinatiUnici(valori)
	var altri []string
	for _, g := range grezzi {
		uguale := false
		for _, v := range valori {
			uguale = uguale || stessaRevisione(g, v)
		}
		if !uguale {
			altri = append(altri, g)
		}
	}
	grezzi = ordinatiUnici(altri)
	e := EvidenzaRevisione{Fonte: fonte}
	switch {
	case discordanti:
		e.Motivo = MotivoEvidenzaLettureDiscordanti
	case len(valori)+len(grezzi) == 0:
		e.Motivo = motivoAssente
	case len(grezzi) > 0:
		e.Motivo = MotivoEvidenzaRevisioneNonInterpretabile
		if len(valori) == 0 && len(grezzi) == 1 {
			g := grezzi[0]
			e.Valore = &g
		}
	case len(valori) > 1:
		e.Motivo = MotivoEvidenzaLettureDiscordanti
	default:
		v := valori[0]
		e.Valore, e.Interpretabile = &v, true
	}
	return e
}

// fonteStepEntita: lo STEP dell'entità come fonte (E1R §5.2, punto 2; T-E1R-05). L'entità: per un 2D associato a un
// componente (un documento confermato, «assegna», o un candidato di un file che ha associazioni decise) i nodi di quel
// componente; per un 2D solo candidato i nodi del suo unico candidato (un ancoraggio candidato_unico); più candidati
// (ambiguo, discordante) danno entita_non_univoca. Più nodi di revisione diversa: entita_non_univoca. La revisione è
// quella interpretata dell'identità del nodo (IdentitaNodo.Revisione), mai rev_grezza, e non passa da un padre ai figli:
// ogni 2D guarda il suo nodo (T-B5-95). La coppia è il grezzo dell'id del nodo (T-B4-22).
func (x *disegniThread) fonteStepEntita(v voceDisegno, componente uuid.UUID) FonteDelDisegno {
	f := FonteDelDisegno{Revisione: EvidenzaRevisione{Fonte: FonteEvidenzaStepEntita, Motivo: MotivoEvidenzaEntitaAssente}}
	var nodi []nodoDellEntita
	switch {
	case v.provenienza != ancoraggio.OrigineProposto:
		nodi = x.nodi[componente]
	case len(x.decisi[v.allegato.ID]) > 0:
		for _, k := range x.decisi[v.allegato.ID] {
			nodi = append(nodi, x.nodi[k]...)
		}
	default:
		a := x.ancoraggi[v.allegato.ID]
		if a == nil || len(a.Candidati) == 0 {
			return f
		}
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || len(a.Candidati) != 1 {
			f.Revisione.Motivo = MotivoEvidenzaEntitaNonUnivoca
			return f
		}
		for _, p := range a.Candidati[0].Posizioni {
			if n, ok := x.nodoPerRif[p.Nodo]; ok {
				nodi = append(nodi, n)
			}
		}
	}
	nodi = nodiUnici(nodi)
	if len(nodi) == 0 {
		return f
	}
	for _, n := range nodi[1:] {
		if testoDi(n.revisione) != testoDi(nodi[0].revisione) || (n.revisione == nil) != (nodi[0].revisione == nil) {
			f.Revisione.Motivo = MotivoEvidenzaEntitaNonUnivoca
			return f
		}
	}
	n := nodi[0]
	f.Revisione = EvidenzaRevisione{Fonte: FonteEvidenzaStepEntita, Entita: n.rif, Valore: copiaTesto(n.revisione), Interpretabile: n.revisione != nil}
	if n.revisione == nil {
		f.Revisione.Motivo = MotivoEvidenzaRevisioneNonDeterminata
	}
	if strings.TrimSpace(n.grezzo.Testo) != "" {
		f.Coppia = ancoraggio.EvidenzaDa(FonteEvidenzaStepEntita, n.grezzo.Testo)
	}
	a := n.allegato
	f.AllegatoID, f.UnitaID, f.Posizione = &a, n.grezzo.UnitaID, n.grezzo.Posizione
	return f
}

// nodiUnici: i nodi senza doppioni, in ordine di Rif.
func nodiUnici(nodi []nodoDellEntita) []nodoDellEntita {
	visti := map[string]bool{}
	var out []nodoDellEntita
	for _, n := range nodi {
		if !visti[n.rif] {
			visti[n.rif] = true
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rif < out[j].rif })
	return out
}

// fonteDocumento: documento.rev e documento.codice come fonte (T-E1R-07): il valore registrato, fuori dalla proposta,
// ultima risorsa della compatibilità; mai un conflitto. Il codice registrato letto con la grammatica (R31 c).
func (x *disegniThread) fonteDocumento(doc *fotorfq.DocumentoConfermato, registrata *string) FonteDelDisegno {
	codice := strings.TrimSpace(testoDi(doc.Codice))
	f := FonteDelDisegno{Revisione: EvidenzaRevisione{Fonte: FonteEvidenzaDocumento, Motivo: MotivoEvidenzaRevisioneNonRegistrata},
		Coppia: ancoraggio.EvidenzaDa(FonteEvidenzaDocumento, codice, testoDi(registrata)), Codice: codice}
	if registrata != nil {
		f.Revisione = EvidenzaRevisione{Fonte: FonteEvidenzaDocumento, Valore: copiaTesto(registrata), Interpretabile: true}
	}
	if l, ok := leggiCodiceRegistrato(x.m, codice); ok {
		f.Lettura = &l
	}
	return f
}

// provenienzaRegistrata: la provenienza di documento.rev (E1R §5.5; T-E1R-07, come T-E1-22 per componente.rev):
// coincide_con_formazione_step se è uguale (senza gli spazi ai bordi) alla formazione di almeno un nodo STEP la cui riga
// legacy decisa porta il componente del documento; altrimenti non_registrata. Delle righe decise legge solo il
// componente, lo sha256 e la chiave; la formazione viene dai fatti (dubbio T-B5-35).
func (x *disegniThread) provenienzaRegistrata(componente uuid.UUID, rev string) string {
	for _, r := range x.t.RigheComponenteProposta {
		if r.ComponenteID == nil || *r.ComponenteID != componente || r.Sha256 == "" || r.Chiave == "" || !x.col.stessoContenuto(r.AllegatoID, r.Sha256) {
			continue
		}
		if r.Stato != statoPropostaConfermata && r.Stato != statoPropostaDuplicato && r.Stato != statoPropostaScartata {
			continue
		}
		for _, f := range x.formazioni[ancoraggio.RifNodo(r.Sha256, r.Chiave)] {
			if strings.TrimSpace(f) == rev {
				return ancoraggio.RevProvenienzaFormazioneSTEP
			}
		}
	}
	return ancoraggio.RevProvenienzaNonRegistrata
}

// revDiversaDellaVista: la rev_diversa di v_fascicolo sul documento, come discordanza documento_componente con
// CalcolataDa = vista (T-E1R-09, E1R §5.4): la riga del componente e del 2D che porta quel documento.
func (x *disegniThread) revDiversaDellaVista(componente, documento uuid.UUID) *DiscordanzaRevisione {
	for _, r := range x.t.Fascicolo {
		if r.ComponenteID == componente && r.TipoDocumento == tipoDocumentoDisegno2D && r.RevDiversa && r.DocumentoID != nil && *r.DocumentoID == documento {
			return &DiscordanzaRevisione{Tra: TraDocumentoComponente, A: strings.TrimSpace(testoDi(r.DocumentoRev)), B: strings.TrimSpace(testoDi(r.Rev)),
				FonteA: FonteEvidenzaDocumento, FonteB: FonteComponenteRegistrata, Effetto: EffettoIndicatore, CalcolataDa: CalcolataDaVista}
		}
	}
	return nil
}

// componenteDaConfrontare: il lato del componente nel criterio 2 (T-B0-34, E1R §5.6): il codice confermato letto con
// la grammatica (R31 c); la revisione dell'identità proposta dai suoi nodi dello STEP, se tutti la danno uguale
// (proposto), altrimenti componente.rev come ultima risorsa (deciso, registrata: R97 B, mai un conflitto). In A1c
// nessuna decisione sul componente arriva qui (LD-27).
func (x *disegniThread) componenteDaConfrontare(k *fotorfq.Componente) ComponenteDaConfrontare {
	c := ComponenteDaConfrontare{ComponenteID: k.ID, Confronto: ConfrontatoConNessuno}
	if l, ok := leggiCodiceRegistrato(x.m, k.Codice); ok {
		c.Lettura = &l
	}
	if nodi := x.nodi[k.ID]; len(nodi) > 0 {
		uguali := nodi[0].revisione != nil
		for _, n := range nodi[1:] {
			uguali = uguali && n.revisione != nil && *n.revisione == *nodi[0].revisione
		}
		if uguali {
			c.Revisione, c.FonteRevisione, c.Confronto = copiaTesto(nodi[0].revisione), FonteComponenteStepEntita, ConfrontatoConProposto
			return c
		}
	}
	if r := strings.TrimSpace(testoDi(k.Rev)); r != "" {
		c.Revisione, c.FonteRevisione, c.Confronto = &r, FonteComponenteRegistrata, ConfrontatoConDeciso
	}
	return c
}

// testoDi: il testo di un puntatore; "" se nil.
func testoDi(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
