package distintabozza

// contenuto-atto-1 (U2 §6.2; [U] X-02, [UR] M27:80): la regola unica di «niente riconferma identica». R4 dice
// uguale_alla_base e differenze_dalla_base[], R5 rifiuta con NESSUNA_MODIFICA (422), con la stessa funzione, chiamata
// dalla funzione condivisa del riepilogo. uguale_alla_base e' vero se e solo se la base e' formato 3 e il delta dell'atto
// rispetto alla base e' vuoto. Una funzione pura: chi chiama costruisce i due contenuti (ContenutoDaSnapshot dallo
// snapshot della conferma di base con la sua mappa, e dalla bozza come R5 la certificherebbe), e aggiunge le parti che
// solo il database sa (archiviazioni, la radice autorizzata e il suo sha, il criterio, le quantita' dei fatti degli archi
// step, le preassociazioni e gli svincoli che R5 applicherebbe adesso, gli effetti sulla working e sulle righe legacy).
//
// Parte 1, lo stato certificato, confrontato come stato:
//   (1) il perimetro: per componente codice e rev byte per byte, tipo, descrizione, archiviazione, la radice con lo STEP
//       autorizzato e il suo sha, l'identita' certificata (emittente, presunzione, namespace);
//   (2) gli archi: padre, figlio, quantita', posizione, provenienza (i valori di trasporto conferma e registrato valgono
//       come la base); per gli archi step la quantita' dei fatti;
//   (3) le esclusioni in vigore, per (sha256, chiave) e motivo (le ereditate valgono come quelle della base);
//   (6) mono_pezzo con il motivo;
//   (7) la fonte (sha256 e radice, mai la chiave del gruppo ne' l'allegato quando il contenuto e' lo stesso) e il criterio;
//   (9) le categorie confermate in vigore, per componente: categoria ed esito.
// Parte 2, cio' che l'atto aggiunge o cambia:
//   (4) il fondamento nuovo: una decisione dell'atto la cui identita', il cui motivo o la cui evidenza citata per contenuto
//       (sha256 del file piu' la copia compatta della lettura senza hash_grammatica, il codice manuale legacy, o
//       un'evidenza documentale) non sono gia' quelli che la base ha certificato per lo stesso componente; una proposta
//       accettata la cui copia compatta non e' gia' quella accettata dalla base; le evidenze documentali dei nodi e degli
//       archi dell'ingegnere, per contenuto;
//   (5) le preassociazioni e gli svincoli che R5 applicherebbe adesso, ciascuno con l'occorrenza (allegato_id) e non con
//       la proposta di documento (direttiva 35, C-2): le voci le scrivono VocePreassociazione e VoceSvincolo;
//   (8) gli effetti che R5 avrebbe adesso sulla working e sulle righe legacy.
// Non entrano: sorgente.importazione e le impronte, gli id delle decisioni, azione_id, chiave_lettura e allegato_id quando
// il contenuto e' lo stesso, orari, autori, chiavi, versioni, base e firma della base, la verifica, gli avvisi, le
// relazioni GP, il vassoio, segnale_visto.versione_servizio, ingresso e da_conferma delle decisioni, il segno
// tipo_confermato (il tipo stesso entra nel punto 1).

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// AlgoritmoContenutoAtto e' il nome della regola.
const AlgoritmoContenutoAtto = "contenuto-atto-1"

// I punti della definizione (U2 §6.2), come li dice differenze_dalla_base[].
const (
	PuntoPerimetro       = 1
	PuntoArchi           = 2
	PuntoEsclusioni      = 3
	PuntoFondamento      = 4
	PuntoPreassociazioni = 5
	PuntoMonoPezzo       = 6
	PuntoFonte           = 7
	PuntoEffetti         = 8
	PuntoCategorie       = 9
)

// ComponenteAtto: un componente del perimetro certificato (punto 1). Chiave: il componente_id, oppure «nuovo:<chiave della
// bozza>» per un nodo da creare.
type ComponenteAtto struct {
	Chiave      string `json:"chiave"`
	Codice      string `json:"codice"`
	Rev         string `json:"rev,omitempty"`
	Tipo        string `json:"tipo"`
	Descrizione string `json:"descrizione,omitempty"`
	Archiviato  bool   `json:"archiviato,omitempty"`
	Radice      bool   `json:"radice,omitempty"`
	// RadiceStepSha: per la radice, lo sha dello STEP autorizzato (da chi chiama)
	RadiceStepSha string `json:"radice_step_sha,omitempty"`
	Emittente     string `json:"emittente"`
	Presunto      bool   `json:"presunto,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
}

// ArcoAtto: un arco certificato (punto 2), fra chiavi di componente.
type ArcoAtto struct {
	Padre     string `json:"padre"`
	Figlio    string `json:"figlio"`
	Qta       int    `json:"qta"`
	Posizione string `json:"posizione,omitempty"`
	// Origine: step o utente; «» per i valori di trasporto (conferma, registrato), che valgono come la base
	Origine  string `json:"origine,omitempty"`
	QtaFatti *int   `json:"qta_fatti,omitempty"`
}

// EsclusioneAtto: un'esclusione in vigore (punto 3): un nodo della fonte non incluso (sha256, chiave) o un componente
// escluso (componente), con il motivo.
type EsclusioneAtto struct {
	Sha256     string `json:"sha256,omitempty"`
	Chiave     string `json:"chiave,omitempty"`
	Componente string `json:"componente,omitempty"`
	Motivo     string `json:"motivo"`
}

// FondamentoAtto: una voce del fondamento (punto 4), per componente (o per arco), confrontata per contenuto.
type FondamentoAtto struct {
	Soggetto  string `json:"soggetto"`
	Contenuto string `json:"contenuto"`
}

// ContenutoAtto: il contenuto di un atto (la conferma di base, o la bozza come R5 la certificherebbe).
type ContenutoAtto struct {
	Componenti        []ComponenteAtto
	Archi             []ArcoAtto
	Esclusioni        []EsclusioneAtto
	MonoPezzo         *string
	FonteSha256       string
	FonteRadice       string
	Criterio          string
	Categorie         map[string]map[string]string // componente → categoria → esito
	Fondamento        []FondamentoAtto
	ProposteAccettate map[string]string // componente → copia compatta dell'identita' proposta accettata
	// solo dell'atto: cio' che R5 applicherebbe o scriverebbe adesso (descrizioni stabili, una per voce; per il punto 5
	// VocePreassociazione e VoceSvincolo)
	Preassociazioni []string
	Effetti         []string
}

// VocePreassociazione: la voce del punto 5 per una preassociazione che R5 applicherebbe,
// «preassociazione:<allegato_id>:<chiave_bozza>» (direttiva 35, C-2: l'occorrenza, mai la proposta).
func VocePreassociazione(p Preassociazione3) string {
	return "preassociazione:" + p.AllegatoID.String() + ":" + p.ChiaveBozza
}

// VoceSvincolo: la voce del punto 5 per uno svincolo (scollega_documento) che R5 applicherebbe,
// «svincolo:<allegato_id>:<componente_id>» (provvisorio, base [T] D-4 A).
func VoceSvincolo(allegato, componente uuid.UUID) string {
	return "svincolo:" + allegato.String() + ":" + componente.String()
}

// DifferenzaAtto: una voce di differenze_dalla_base[] (firmata da R4).
type DifferenzaAtto struct {
	Punto  int    `json:"punto"`
	Chiave string `json:"chiave"`
	Testo  string `json:"testo"`
}

// ContenutoAtto1 confronta l'atto con la base (commento di testa). baseFormato3 falso (nessuna base, o una base formato 1 o
// 2): uguale e' sempre falso, e le differenze non si calcolano (la prima conferma formato 3 e' sempre ammessa).
func ContenutoAtto1(base ContenutoAtto, baseFormato3 bool, atto ContenutoAtto) (bool, []DifferenzaAtto) {
	if !baseFormato3 {
		return false, nil
	}
	var dd []DifferenzaAtto
	add := func(p int, k, f string, a ...any) {
		dd = append(dd, DifferenzaAtto{Punto: p, Chiave: k, Testo: fmt.Sprintf(f, a...)})
	}

	// (1) il perimetro
	bc := map[string]ComponenteAtto{}
	for _, c := range base.Componenti {
		bc[c.Chiave] = c
	}
	ac := map[string]ComponenteAtto{}
	for _, c := range atto.Componenti {
		ac[c.Chiave] = c
	}
	for _, k := range chiaviUnite(ac, bc) {
		a, inA := ac[k]
		b, inB := bc[k]
		switch {
		case !inB:
			add(PuntoPerimetro, k, "componente nuovo nel perimetro: %s", a.Codice)
		case !inA:
			add(PuntoPerimetro, k, "componente tolto dal perimetro: %s", b.Codice)
		case a != b:
			add(PuntoPerimetro, k, "componente cambiato: %s", campiDiversi(b, a))
		}
	}
	// (2) gli archi
	ba := map[string]ArcoAtto{}
	for _, x := range base.Archi {
		ba[x.Padre+"→"+x.Figlio] = x
	}
	aa := map[string]ArcoAtto{}
	for _, x := range atto.Archi {
		aa[x.Padre+"→"+x.Figlio] = x
	}
	for _, k := range chiaviUnite(aa, ba) {
		a, inA := aa[k]
		b, inB := ba[k]
		switch {
		case !inB:
			add(PuntoArchi, k, "arco nuovo")
		case !inA:
			add(PuntoArchi, k, "arco tolto")
		default:
			if a.Origine == "" {
				a.Origine = b.Origine // i valori di trasporto valgono come la base
			}
			if b.Origine == "" {
				b.Origine = a.Origine
			}
			if a.Qta != b.Qta || a.Posizione != b.Posizione || a.Origine != b.Origine || !stessoIntero(a.QtaFatti, b.QtaFatti) {
				add(PuntoArchi, k, "arco cambiato (quantita', posizione, provenienza o quantita' dei fatti)")
			}
		}
	}
	// (3) le esclusioni
	chiaveEsc := func(e EsclusioneAtto) string {
		if e.Componente != "" {
			return "componente:" + e.Componente
		}
		return "nodo:" + e.Sha256 + ":" + e.Chiave
	}
	be := map[string]string{}
	for _, e := range base.Esclusioni {
		be[chiaveEsc(e)] = e.Motivo
	}
	ae := map[string]string{}
	for _, e := range atto.Esclusioni {
		ae[chiaveEsc(e)] = e.Motivo
	}
	for _, k := range chiaviUnite(ae, be) {
		a, inA := ae[k]
		b, inB := be[k]
		switch {
		case !inB:
			add(PuntoEsclusioni, k, "esclusione nuova")
		case !inA:
			add(PuntoEsclusioni, k, "esclusione tolta")
		case a != b:
			add(PuntoEsclusioni, k, "motivo dell'esclusione cambiato")
		}
	}
	// (6) mono_pezzo
	if (base.MonoPezzo == nil) != (atto.MonoPezzo == nil) || (base.MonoPezzo != nil && *base.MonoPezzo != *atto.MonoPezzo) {
		add(PuntoMonoPezzo, "mono_pezzo", "dichiarazione mono_pezzo cambiata")
	}
	// (7) la fonte e il criterio
	if base.FonteSha256 != atto.FonteSha256 || base.FonteRadice != atto.FonteRadice {
		add(PuntoFonte, "fonte", "la fonte (sha256 e radice) e' cambiata")
	}
	if base.Criterio != atto.Criterio {
		add(PuntoFonte, "criterio", "il criterio e' cambiato: %s → %s", base.Criterio, atto.Criterio)
	}
	// (9) le categorie in vigore
	bk := appiattisci(base.Categorie)
	ak := appiattisci(atto.Categorie)
	for _, k := range chiaviUnite(ak, bk) {
		if ak[k] != bk[k] {
			add(PuntoCategorie, k, "categoria confermata cambiata: %q → %q", bk[k], ak[k])
		}
	}
	// (4) il fondamento nuovo: solo cio' che l'atto aggiunge rispetto a quello che la base ha certificato
	bf := map[string]bool{}
	for _, f := range base.Fondamento {
		bf[f.Soggetto+"\x00"+f.Contenuto] = true
	}
	var nuovi []string
	visti := map[string]bool{}
	for _, f := range atto.Fondamento {
		k := f.Soggetto + "\x00" + f.Contenuto
		if !bf[k] && !visti[k] {
			visti[k] = true
			nuovi = append(nuovi, f.Soggetto)
		}
	}
	sort.Strings(nuovi)
	for _, s := range nuovi {
		add(PuntoFondamento, s, "fondamento nuovo (identita', motivo o evidenza)")
	}
	for _, k := range chiaviUnite(atto.ProposteAccettate, map[string]string(nil)) {
		if base.ProposteAccettate[k] != atto.ProposteAccettate[k] {
			add(PuntoFondamento, k, "proposta accettata nuova")
		}
	}
	// (5) e (8): cio' che R5 applicherebbe adesso
	for _, p := range ordinatiUnici(atto.Preassociazioni) {
		add(PuntoPreassociazioni, p, "preassociazione o svincolo da applicare")
	}
	for _, e := range ordinatiUnici(atto.Effetti) {
		add(PuntoEffetti, e, "effetto sulla working o sulle righe")
	}
	sort.SliceStable(dd, func(i, j int) bool {
		if dd[i].Punto != dd[j].Punto {
			return dd[i].Punto < dd[j].Punto
		}
		return dd[i].Chiave < dd[j].Chiave
	})
	if dd == nil {
		dd = []DifferenzaAtto{}
	}
	return len(dd) == 0, dd
}

func stessoIntero(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func appiattisci(m map[string]map[string]string) map[string]string {
	out := map[string]string{}
	for c, cc := range m {
		for cat, esito := range cc {
			out[c+":"+cat] = esito
		}
	}
	return out
}

func ordinatiUnici(xx []string) []string {
	m := map[string]bool{}
	for _, x := range xx {
		m[x] = true
	}
	out := make([]string, 0, len(m))
	for x := range m {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

// chiaviUnite: le chiavi di due mappe, unite e in ordine.
func chiaviUnite[V any, W any](a map[string]V, b map[string]W) []string {
	m := map[string]bool{}
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func campiDiversi(b, a ComponenteAtto) string {
	var cc []string
	if a.Codice != b.Codice || a.Rev != b.Rev {
		cc = append(cc, "codice o revisione")
	}
	if a.Tipo != b.Tipo {
		cc = append(cc, "tipo")
	}
	if a.Descrizione != b.Descrizione {
		cc = append(cc, "descrizione")
	}
	if a.Archiviato != b.Archiviato {
		cc = append(cc, "archiviazione")
	}
	if a.Radice != b.Radice || a.RadiceStepSha != b.RadiceStepSha {
		cc = append(cc, "radice o STEP autorizzato")
	}
	if a.Emittente != b.Emittente || a.Presunto != b.Presunto || a.Namespace != b.Namespace {
		cc = append(cc, "identita' certificata")
	}
	return strings.Join(cc, ", ")
}

// ------------------------------------------------------------------ dallo snapshot

// ContenutoDaSnapshot costruisce le parti del contenuto che lo snapshot formato 3 dice da solo (perimetro senza
// archiviazioni ne' sha dello STEP autorizzato, archi senza quantita' dei fatti, esclusioni, mono_pezzo, fonte, categorie,
// fondamento, proposte accettate). mappa: la chiave della bozza → il componente (la mappa della conferma, o la
// risoluzione di R5); un nodo con componente_id usa il suo. Chi chiama aggiunge archiviazioni, la radice autorizzata,
// il criterio, le quantita' dei fatti, le preassociazioni e gli effetti.
func ContenutoDaSnapshot(s Snapshot3, mappa map[string]uuid.UUID) ContenutoAtto {
	out := ContenutoAtto{Categorie: map[string]map[string]string{}, ProposteAccettate: map[string]string{}}
	chiave := map[string]string{}
	for _, n := range s.Nodi {
		switch {
		case n.ComponenteID != nil:
			chiave[n.ChiaveBozza] = n.ComponenteID.String()
		case mappa[n.ChiaveBozza] != uuid.Nil:
			chiave[n.ChiaveBozza] = mappa[n.ChiaveBozza].String()
		default:
			chiave[n.ChiaveBozza] = "nuovo:" + n.ChiaveBozza
		}
	}
	perID := map[string]Decisione3{}
	for _, d := range s.Decisioni {
		perID[d.ID] = d
	}
	for _, n := range s.Nodi {
		if n.Op == OpEsclude {
			continue // un registrato escluso esce dal perimetro: e' un'esclusione (punto 3)
		}
		k := chiave[n.ChiaveBozza]
		radice := n.ComponenteID != nil && *n.ComponenteID == s.ProdottoID
		out.Componenti = append(out.Componenti, ComponenteAtto{Chiave: k, Codice: n.Identita.Codice, Rev: n.Identita.Rev, Tipo: n.Tipo,
			Descrizione: n.Descrizione, Radice: radice, Emittente: n.Identita.Emittente, Presunto: n.Identita.EmittentePresunto,
			Namespace: n.Identita.Namespace})
		if radice && len(n.Caselle) > 0 {
			out.FonteRadice = n.Caselle[0].Rif
		}
		if n.Identita.Modo == ModoProposta {
			out.ProposteAccettate[k] = identitaCompatta(n.Identita.Emittente, n.Identita.Namespace, n.Identita.Codice, n.Identita.Rev)
		}
		for _, e := range n.EvidenzeIdentita {
			out.Fondamento = append(out.Fondamento, FondamentoAtto{Soggetto: k, Contenuto: "evidenza:" + evidenzaPerContenuto(e)})
		}
	}
	for _, a := range s.Archi {
		x := ArcoAtto{Padre: chiave[a.Padre], Figlio: chiave[a.Figlio], Qta: a.Qta, Posizione: a.Posizione}
		if a.Origine == OrigineStep || a.Origine == OrigineUtente {
			x.Origine = a.Origine
		}
		out.Archi = append(out.Archi, x)
		for _, e := range a.EvidenzeArco {
			out.Fondamento = append(out.Fondamento, FondamentoAtto{Soggetto: x.Padre + "→" + x.Figlio, Contenuto: "evidenza:" + evidenzaPerContenuto(e)})
		}
	}
	if s.Sorgente != nil {
		out.FonteSha256 = s.Sorgente.Sha256
	}
	for _, d := range s.Decisioni {
		switch d.Tipo {
		case DecisioneNonIncludere:
			sha, ch := shaEChiave(d.Casella.Rif)
			out.Esclusioni = append(out.Esclusioni, EsclusioneAtto{Sha256: sha, Chiave: ch, Motivo: d.MotivoCodice + ":" + d.Motivo})
		case DecisioneEscludi:
			out.Esclusioni = append(out.Esclusioni, EsclusioneAtto{Componente: d.ComponenteID.String(), Motivo: d.Motivo})
		case DecisioneMonoPezzo:
			m := d.Motivo
			out.MonoPezzo = &m
		case DecisioneConfermaCategoria:
			k := chiave[d.Soggetto.ChiaveBozza]
			if out.Categorie[k] == nil {
				out.Categorie[k] = map[string]string{}
			}
			out.Categorie[k][d.Categoria] = d.Esito
		case DecisioneIdentita:
			out.Fondamento = append(out.Fondamento, FondamentoAtto{Soggetto: chiave[d.ChiaveBozza], Contenuto: fondamentoIdentita(d)})
		case DecisioneRicodifica:
			out.Fondamento = append(out.Fondamento, FondamentoAtto{Soggetto: d.ComponenteID.String(),
				Contenuto: "ricodifica:" + identitaCompatta(d.EmittenteDopo, d.NamespaceDopo, d.CodiceDopo, "") + "|" + d.Motivo})
		case DecisioneCambiaTipo:
			out.Fondamento = append(out.Fondamento, FondamentoAtto{Soggetto: d.ComponenteID.String(), Contenuto: "tipo:" + d.TipoNuovo})
		}
	}
	return out
}

func identitaCompatta(emittente, namespace, codice, rev string) string {
	return strings.Join([]string{emittente, namespace, codice, rev}, "\x1f")
}

// fondamentoIdentita: il contenuto di una decisione identita (U2 §6.2 punto 4): l'identita', il motivo e l'evidenza per
// contenuto (lo sha256 del file della lettura piu' la copia compatta senza hash_grammatica; il codice manuale legacy;
// l'evidenza documentale), mai chiave_lettura, allegato_id o importazione.
func fondamentoIdentita(d Decisione3) string {
	parti := []string{"identita", identitaCompatta(d.Identita.Emittente, d.Identita.Namespace, d.Identita.Codice, d.Identita.Rev), d.Modo, d.Motivo}
	if d.Lettura != nil {
		parti = append(parti, "lettura:"+letturaSenzaHash(d.Lettura))
	}
	if d.Riferimento != "" {
		parti = append(parti, "riferimento:"+d.Riferimento)
	}
	if d.EvidenzaDocumentale != nil {
		parti = append(parti, "evidenza:"+evidenzaPerContenuto(*d.EvidenzaDocumentale))
	}
	return strings.Join(parti, "|")
}

// letturaSenzaHash: la copia compatta della lettura senza hash_grammatica (U2-P15, B-1 strada (a)), in forma canonica.
func letturaSenzaHash(l *importazione.LetturaCompatta) string {
	b, err := json.Marshal(l)
	if err != nil {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return string(b)
	}
	delete(m, "hash_grammatica")
	delete(m, "chiave_lettura")
	c, err := jsoncanonico.Codifica(m)
	if err != nil {
		return string(b)
	}
	return string(c)
}

// evidenzaPerContenuto: un'evidenza documentale per contenuto: lo sha256 del file, le pagine, la riga, la nota (mai
// l'allegato: lo stesso contenuto in un'altra occorrenza e' la stessa evidenza).
func evidenzaPerContenuto(e EvidenzaDocumentale) string {
	pp := make([]string, len(e.Pagine))
	for i, p := range e.Pagine {
		pp[i] = strconv.Itoa(p)
	}
	return strings.Join([]string{e.Sha256, strings.Join(pp, ","), e.Riga, e.Nota}, "\x1f")
}
