package distintabozza

// R2, forma cambia_sorgente, algoritmo cambio-fonte-1 (EM §6.6; U2 §5.2 con la base B di V1-Q30; MAT §4.3 riga 12; U2 §2.8;
// direttiva 35, blocco C-3). Il gruppo dichiarato si sceglie prima della transazione fra quelli del prodotto
// (GruppoDaImportare, con stessa_sorgente), il registro e' gia' confrontato con l'impronta dichiarata (V-7).
//
// Gli elenchi della risposta sono quelli di propostabom.EffettoDelCambioFonte, la stessa funzione dell'anteprima di S6
// (`?bozza=`): fuori_per_cambio_fonte, nuovi_nodi_proposti, caselle_riscritte, archi_riscritti, identita_aggiornate,
// da_rivedere, decisioni_cadute, differenze; la versione n+1 li applica (Cambia):
//  1. la sorgente prende il registro nuovo (allegato, sha256, gruppo, impronta);
//  2. il nodo del prodotto resta, con la casella della radice nuova; la sua identita' non cambia;
//  3. un nodo con la casella della fonte vecchia la cui voce c'e' nel registro nuovo (per rif) resta, con la casella
//     riscritta sull'occorrenza e sul gruppo nuovi; un nodo proposta la cui identita' cambia vuole la scelta recepisci /
//     mantieni (base B di V1-Q30), un da_decidere la prende da solo; un nodo deciso resta com'era;
//  4. gli altri nodi della fonte vecchia escono (nodo_tolto), con i loro archi; le loro preassociazioni vanno nel vassoio
//     (file_svincolato, motivo fonte_cambiata). Nessun ricollegamento automatico per codice;
//  5. i nodi senza casella e i registrati restano; gli archi step fra nodi che restano prendono lo step nuovo;
//  6. le voci nuove non rappresentate entrano come nella partenza step (P-1…P-5; P-6 per il gruppo bom fuori dal caso delle
//     due occorrenze della stessa struttura); fra due occorrenze della stessa struttura nessuna voce entra da sola
//     (nuovi_nodi_proposti);
//  7. le decisioni che citano nodi usciti cadono (decisione_tolta), con le non_includere su un rif che il registro nuovo
//     non ha; una non_includere che resta prende la casella sull'occorrenza e sul gruppo nuovi; le decisioni identita non
//     si riscrivono;
//  8. eventi: quelli del server dei passi 4 e 7, per ultimo sorgente_cambiata (prima la sorgente di prima; dopo la nuova
//     con algoritmo, conteggi, da_rivedere[] e le chiavi degli altri elenchi; il motivo); fonte_step_evento sostituzione
//     con precedente_* (scelta, se la bozza non aveva una fonte STEP);
//  9. la base non cambia.
// Direttiva 39 §4 (pbiniziali3.go; D-PB01 A, D-PB02 A [U]), che supera «nessuna PA diventa PB» ([U] dir. 35 §8 C-3) per
// le proposte univoche: dopo la versione n+1 le proposte univoche del registro nuovo diventano PB motore (eventi
// file_preassociato del server, prima di sorgente_cambiata), solo con la sorgente confermata da S8 (D-AR11 B [U]); le PB
// rimaste tengono provenienza e registro d'origine (D-PB03 A); gli altri suggerimenti in preassociazioni_proposte[].
//
// Da una versione senza struttura (nessuna sorgente, o la riservata solo_prodotto: SenzaStruttura) vale cambio-fonte-2
// (direttiva 39 §3; riconciliazione3.go): gli stessi passi con la riconciliazione dei nodi senza casella (3-bis, 3-ter),
// le regole degli archi e delle decisioni di RiconciliaEffetto, niente P-6; la sorgente solo_prodotto vale come assenza
// per l'effetto. Fra due sorgenti STEP cambio-fonte-1 resta com'era, salvo la revisione di una versione con una base
// confermata: i suoi nodi registrati che il registro nuovo non ha per rif (lo sha256 e' cambiato) si riconciliano per
// identita' con le regole di cambio-fonte-2 invece di uscire (riconciliazione3.go, preparaRevisione) [T-39].

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/rfq/traccia"
	"promatec/cockpit/internal/platform/db"
)

// CambioFonte3: una richiesta della forma cambia_sorgente, gia' letta, con la RFQ, l'utente e la proposta calcolata.
type CambioFonte3 struct {
	ThreadID, UtenteID uuid.UUID
	Comune             ComuneR2
	CambiaSorgente     CambiaSorgenteR2
	Proposta           *PropostaCalcolata
	// PrimaDelCommit: come Creazione3.PrimaDelCommit (S8, U2 §2.2).
	PrimaDelCommit func(ctx context.Context, q *db.Queries, bozza uuid.UUID, versione int, imp *ImportazioneR2) error
}

// CambiaSorgenteDaRichiesta: la CambioFonte3 di un corpo di R2 nella forma cambia_sorgente.
func (r RichiestaR2) CambiaSorgenteDaRichiesta(thread, utente uuid.UUID, p *PropostaCalcolata) CambioFonte3 {
	x := CambioFonte3{ThreadID: thread, UtenteID: utente, Comune: r.Comune, Proposta: p}
	if r.CambiaSorgente != nil {
		x.CambiaSorgente = *r.CambiaSorgente
	}
	return x
}

// EsitoCambioFonte: la versione n+1 di cambio-fonte-1, i suoi eventi del server e gli elenchi.
type EsitoCambioFonte struct {
	Snapshot   Snapshot3
	eventi     []eventoServer
	Effetto    propostabom.EffettoCambioFonte
	Aggiornate []IdentitaAggiornata3
	Scritte    []string
	Vassoio    []string
	Entrati    []string
	// Riconciliazione: cambio-fonte-2 (riconciliazione3.go), per una versione senza struttura o nella revisione con un
	// secondo STEP; nil con cambio-fonte-1
	Riconciliazione *RiconciliazioneFonte3
	// Ricollegati: nella revisione, i nodi registrati della fonte di prima riconciliati per identita' (preparaRevisione)
	Ricollegati []string
}

// VistaDellaBozza: della bozza formato 3, la vista che propostabom.EffettoDelCambioFonte legge (le preassociazioni non
// servono: il vassoio lo calcola chi applica il cambio).
func VistaDellaBozza(s Snapshot3, radice string) propostabom.VistaBozza {
	var v propostabom.VistaBozza
	if s.Sorgente != nil && s.Sorgente.AllegatoID != nil {
		v.Sorgente = propostabom.SorgenteBozza{AllegatoID: *s.Sorgente.AllegatoID, Sha256: s.Sorgente.Sha256, Radice: radice, Gruppo: s.Sorgente.Gruppo}
	}
	for _, n := range s.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == s.ProdottoID {
			v.NodoProdotto = n.ChiaveBozza
		}
		x := propostabom.NodoBozza{ChiaveBozza: n.ChiaveBozza, Identita: propostabom.IdentitaBozza{Emittente: n.Identita.Emittente,
			Namespace: n.Identita.Namespace, Codice: n.Identita.Codice, Rev: n.Identita.Rev, Modo: n.Identita.Modo, Decisione: n.Identita.Decisione}}
		for _, c := range n.Caselle {
			x.Caselle = append(x.Caselle, propostabom.CasellaBozza{AllegatoID: c.AllegatoID, Gruppo: c.Gruppo, Rif: c.Rif})
		}
		v.Nodi = append(v.Nodi, x)
	}
	for _, a := range s.Archi {
		x := propostabom.ArcoBozza{Padre: a.Padre, Figlio: a.Figlio, Origine: a.Origine}
		if a.Step != nil {
			x.Step = &propostabom.PassoStep{AllegatoID: a.Step.AllegatoID, Gruppo: a.Step.Gruppo}
		}
		v.Archi = append(v.Archi, x)
	}
	for _, d := range s.Decisioni {
		x := propostabom.DecisioneBozza{ID: d.ID, Tipo: d.Tipo, ChiaveLettura: d.ChiaveLettura, Lettura: d.Lettura}
		if d.ChiaveBozza != "" {
			x.CitaNodi = append(x.CitaNodi, d.ChiaveBozza)
		}
		if d.Soggetto != nil && d.Soggetto.ChiaveBozza != "" {
			x.CitaNodi = append(x.CitaNodi, d.Soggetto.ChiaveBozza)
		}
		if d.Casella != nil {
			x.Casella = &propostabom.CasellaBozza{AllegatoID: d.Casella.AllegatoID, Gruppo: d.Casella.Gruppo, Rif: d.Casella.Rif}
		}
		v.Decisioni = append(v.Decisioni, x)
	}
	return v
}

// radiceDellaSorgente: la radice della sorgente di una versione (fonte_radice del suo registro, altrimenti il rif della
// casella del nodo del prodotto; "" senza: una bozza venuta dal formato 2).
func radiceDellaSorgente(s Snapshot3, reg *importazione.Registro) string {
	if reg != nil && reg.Contenuto.Gruppo.Radice != "" {
		return reg.Contenuto.Gruppo.Radice
	}
	for _, n := range s.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == s.ProdottoID && len(n.Caselle) > 0 {
			return n.Caselle[0].Rif
		}
	}
	return ""
}

// Cambia costruisce la versione n+1 di cambio-fonte-1 (commento di testa). vecchio e' il registro della versione (nil
// senza), nuovo quello del gruppo dichiarato.
func Cambia(prima Snapshot3, vecchio *importazione.Registro, nuovo importazione.Registro, scelte []SceltaR2, motivo string, in IngressoPartenza) (EsitoCambioFonte, error) {
	var out EsitoCambioFonte
	nc := nuovo.Contenuto
	allegato, err := uuid.Parse(nc.Proposta.Fonte.AllegatoID)
	if err != nil {
		return out, corpoMotivo("cambia_sorgente", MotivoGruppoNonImportabile, "il registro nuovo non ha la fonte")
	}
	radice := radiceDellaSorgente(prima, vecchio)
	// la revisione con un secondo STEP (riconciliazione3.go, preparaRevisione): i nodi registrati che il registro nuovo
	// non ha per rif si riconciliano per identita' come i nodi senza casella di cambio-fonte-2
	rev, revisione := preparaRevisione(prima, nc)
	if revisione {
		prima = rev.snapshot
		out.Ricollegati = make([]string, 0, len(rev.origini))
		for k := range rev.origini {
			out.Ricollegati = append(out.Ricollegati, k)
		}
		sort.Strings(out.Ricollegati)
	}
	vista := VistaDellaBozza(prima, radice)
	if SenzaStruttura(prima) {
		// la sorgente solo_prodotto non ha struttura: per l'effetto e' come l'assenza (le voci dello stesso STEP, ora
		// analizzato, entrano come prima struttura; non e' il caso delle due occorrenze)
		vista.Sorgente = propostabom.SorgenteBozza{}
	}
	var vc *importazione.Contenuto
	if vecchio != nil {
		vc = &vecchio.Contenuto
	}
	eff := propostabom.EffettoDelCambioFonte(vista, vc, nc)
	if eff.StessaSorgente {
		return out, corpoMotivo("cambia_sorgente.sorgente.gruppo", MotivoStessaSorgente,
			"la sorgente dichiarata e' la stessa struttura della bozza, sulla stessa occorrenza: e' un riallineamento («Aggiorna alla proposta di oggi»)")
	}
	if SenzaStruttura(prima) || revisione {
		// cambio-fonte-2: la bozza senza struttura (vuota, da una conferma senza fonte, solo_prodotto) si riconcilia; nella
		// revisione anche i nodi registrati della fonte di prima
		ric, err := RiconciliaEffetto(prima, nc, eff)
		if err != nil {
			return out, err
		}
		eff = ric.Effetto
		out.Riconciliazione = &ric
	}
	out.Effetto = eff
	gruppo := nc.Gruppo.Chiave
	c := nuovoCantiere(prima, scelte, motivo)
	c.s.Sorgente = &Sorgente3{AllegatoID: &allegato, Sha256: nc.Proposta.Fonte.Sha256, Gruppo: gruppo, Importazione: nuovo.Impronta}
	var allegatoVecchio uuid.UUID
	if prima.Sorgente != nil && prima.Sorgente.AllegatoID != nil {
		allegatoVecchio = *prima.Sorgente.AllegatoID
	}
	// passo 4: i nodi che escono
	for _, k := range eff.FuoriPerCambioFonte {
		c.togliNodo(k, SvincoloFonteCambiata)
	}
	// passi 2 e 3: il nodo del prodotto e i nodi che restano con la casella riscritta
	aggiornate := map[string]bool{}
	for _, x := range eff.IdentitaAggiornate {
		aggiornate[x.ChiaveBozza] = true
	}
	for i := range c.s.Nodi {
		n := &c.s.Nodi[i]
		if n.ComponenteID != nil && *n.ComponenteID == prima.ProdottoID {
			if nc.Gruppo.Radice != "" {
				n.Caselle = []Casella3{{AllegatoID: allegato, Gruppo: gruppo, Rif: nc.Gruppo.Radice}}
			} else {
				n.Caselle = nil
			}
			continue
		}
		if len(n.Caselle) == 0 {
			continue
		}
		rif := n.Caselle[0].Rif
		n.Caselle = []Casella3{{AllegatoID: allegato, Gruppo: gruppo, Rif: rif}}
		if !aggiornate[n.ChiaveBozza] {
			continue
		}
		var nuova Identita
		if v, ok := nc.VocePerRif(allegato, rif); ok {
			nuova, err = identitaNuovaDellaVoce(v, in.Cliente)
		} else {
			x, _ := nc.FuoriPerRif(allegato, rif)
			nuova, err = identitaDellaVoceFuori(x, in.Cliente)
		}
		if err != nil {
			return out, err
		}
		if c.s.Nodi[i].Identita.Modo == ModoProposta && stessaIdentita3(c.s.Nodi[i].Identita, nuova) {
			continue
		}
		if err := c.aggiornaIdentitaAnche(i, nuova, vecchio, allegatoVecchio, rif); err != nil {
			return out, err
		}
	}
	// passo 3-bis (cambio-fonte-2): i nodi senza casella abbinati prendono la casella della voce, con la stessa chiave
	if r := out.Riconciliazione; r != nil {
		for _, a := range r.Abbinati {
			i := indiceNodo(c.s, a.ChiaveBozza)
			if i < 0 {
				return out, fmt.Errorf("il nodo abbinato %s non e' nella bozza", a.ChiaveBozza)
			}
			n := &c.s.Nodi[i]
			n.Caselle = []Casella3{{AllegatoID: allegato, Gruppo: gruppo, Rif: a.Rif}}
			if a.identita != nil && !stessaIdentita3(n.Identita, *a.identita) {
				c.aggiornate = append(c.aggiornate, IdentitaAggiornata3{ChiaveBozza: n.ChiaveBozza, Prima: n.Identita, Dopo: *a.identita})
				n.Identita = *a.identita
			}
		}
	}
	// passo 5: gli archi step che restano
	for i := range c.s.Archi {
		if c.s.Archi[i].Step != nil {
			c.s.Archi[i].Step = &StepArco3{AllegatoID: allegato, Gruppo: gruppo}
		}
	}
	// passo 7: le decisioni che cadono (quelle dei nodi usciti sono gia' tolte) e le non_includere che restano
	for _, id := range eff.DecisioniCadute {
		c.togliDecisione(id)
	}
	for j := range c.s.Decisioni {
		d := &c.s.Decisioni[j]
		if d.Tipo == DecisioneNonIncludere && d.Casella != nil {
			x := *d.Casella
			x.AllegatoID, x.Gruppo = allegato, gruppo
			d.Casella = &x
		}
	}
	// passo 6: le voci nuove che entrano, con i loro archi (P-5: solo quelli che toccano una voce entrata) e P-6
	entra := map[string]bool{}
	for _, v := range eff.NodiCheEntrano {
		entra[v] = true
	}
	chiaveDi := map[string]string{}
	for _, n := range c.s.Nodi {
		for _, x := range n.Caselle {
			chiaveDi[x.Rif] = n.ChiaveBozza
		}
	}
	var tr TracciaPartenza
	registrati := map[uuid.UUID]string{}
	for _, n := range c.s.Nodi {
		if n.ComponenteID != nil {
			registrati[*n.ComponenteID] = n.ChiaveBozza
		}
	}
	entrati := map[string]bool{}
	for _, v := range nc.Proposta.Nodi {
		if !entra[v.Voce] {
			continue
		}
		var n Nodo3
		if comp, ok := decisioneEsistente(v); ok && comp != in.Prodotto {
			if w, ok := in.Stato.Componente(comp); ok && !w.Archiviato {
				n = in.nodoRegistrato(w, ChiaveDellaVoce(allegato, v.Origine.Rif), OrigineStep, &tr)
				registrati[comp] = n.ChiaveBozza
			}
		}
		if n.ChiaveBozza == "" {
			x, err := nodoProposto(v, allegato, nc, in.Cliente, &tr)
			if err != nil {
				return out, err
			}
			if comp, ok := decisioneEsistente(v); ok && comp == in.Prodotto {
				x.Tipo = "sottoassieme"
			}
			n = x
		}
		n.Caselle = []Casella3{{AllegatoID: allegato, Gruppo: gruppo, Rif: v.Origine.Rif}}
		if indiceNodo(c.s, n.ChiaveBozza) >= 0 {
			return out, fmt.Errorf("la chiave %s del nodo che entra e' gia' nella bozza", n.ChiaveBozza)
		}
		c.s.Nodi = append(c.s.Nodi, n)
		chiaveDi[v.Origine.Rif] = n.ChiaveBozza
		entrati[v.Origine.Rif] = true
		out.Entrati = append(out.Entrati, n.ChiaveBozza)
	}
	nellaBozza := map[[2]string]bool{}
	for _, a := range c.s.Archi {
		nellaBozza[[2]string{a.Padre, a.Figlio}] = true
	}
	archiCheEntrano := nc.Proposta.Archi
	if r := out.Riconciliazione; r != nil {
		// cambio-fonte-2: gli archi li ha scelti RiconciliaEffetto (P-5 e l'arco verso un abbinato senza padre)
		archiCheEntrano = nil
		for _, a := range r.archi {
			archiCheEntrano = append(archiCheEntrano, importazione.ArcoRegistro{AllegatoID: allegato.String(), Padre: a.padre, Figlio: a.figlio, Quantita: a.qta})
		}
	}
	for _, a := range archiCheEntrano {
		if out.Riconciliazione == nil && !entrati[a.Padre] && !entrati[a.Figlio] {
			continue
		}
		kp, okp := chiaveDi[a.Padre]
		kf, okf := chiaveDi[a.Figlio]
		if !okp || !okf || a.AllegatoID != allegato.String() || nellaBozza[[2]string{kp, kf}] {
			continue
		}
		if a.Quantita == nil || *a.Quantita < 1 || *a.Quantita > MaxQtaArco {
			return out, corpoMotivo("cambia_sorgente", MotivoArcoSenzaQuantita,
				"l'arco %s → %s del registro non ha una quantita' fra 1 e %d: la versione non la inventa", a.Padre, a.Figlio, MaxQtaArco)
		}
		c.s.Archi = append(c.s.Archi, Arco3{Padre: kp, Figlio: kf, Qta: *a.Quantita, Origine: OrigineStep, Step: &StepArco3{AllegatoID: allegato, Gruppo: gruppo}})
		nellaBozza[[2]string{kp, kf}] = true
	}
	if len(eff.NodiCheEntrano) > 0 && gruppo == GruppoBOMDiLavoro && out.Riconciliazione == nil {
		if err := in.confermatiSenzaStep(&c.s, registrati, &tr); err != nil {
			return out, err
		}
	}
	if revisione {
		rev.dopoLaRevisione(&c.s, out.Riconciliazione, nc, allegato, gruppo)
	}
	if e := c.controllaScelte("cambia_sorgente.scelte", scelte); e != nil {
		return out, e
	}
	c.chiude()
	out.Snapshot, out.eventi, out.Aggiornate, out.Scritte, out.Vassoio = c.s, c.eventi, c.aggiornate, c.scritte, c.vassoio
	return out, nil
}

// aggiornaIdentitaAnche: come aggiornaIdentita, anche quando le due identita' differiscono solo dove l'effetto non guarda
// (l'emittente vuoto da una parte): l'effetto ha gia' detto che l'identita' cambia.
func (c *cantiere) aggiornaIdentitaAnche(i int, nuova Identita, vecchio *importazione.Registro, allegatoVecchio uuid.UUID, rif string) error {
	n := &c.s.Nodi[i]
	switch n.Identita.Modo {
	case ModoDaDecidere:
		c.aggiornate = append(c.aggiornate, IdentitaAggiornata3{ChiaveBozza: n.ChiaveBozza, Prima: n.Identita, Dopo: nuova})
		n.Identita = nuova
		return nil
	case ModoProposta:
		switch c.scelta(n.ChiaveBozza, "") {
		case SceltaRecepisci:
			c.aggiornate = append(c.aggiornate, IdentitaAggiornata3{ChiaveBozza: n.ChiaveBozza, Prima: n.Identita, Dopo: nuova})
			n.Identita = nuova
		case SceltaMantieni:
			return c.mantieni(i, vecchio, allegatoVecchio, rif)
		default:
			c.manca(n.ChiaveBozza + " (recepisci o mantieni)")
		}
	}
	return nil
}

// CambiaSorgenteFormato3: R2 cambia_sorgente (commento di testa). L'ordine: BloccaThread → la chiave → il prodotto → la
// bozza (aperta, alla versione attesa, formato 3) → il registro nuovo e V-7 → la fonte confermata (FONTE_NON_CONFERMATA) →
// le occorrenze del registro → cambio-fonte-1 → il registro trovato o scritto, la versione n+1, gli eventi,
// fonte_step_evento.
// WidgetCambioFonte: il gesto della pagina che la traccia del cambio della fonte nomina (S8 passa la sua traccia).
const WidgetCambioFonte = "Usa nuovo STEP"

// tracciaCambioFonte: la traccia del cambio della fonte senza il motivo dell'utente (messaggio 46 [U]), per esempio
// «gesto di PS da «Usa nuovo STEP»: la bozza di ACME-1 passa allo STEP «ACME-1 rev B.stp»».
func tracciaCambioFonte(ctx context.Context, q *db.Queries, thread, utente, allegato uuid.UUID, codiceProdotto string) (string, error) {
	nome := allegato.String()
	if a, err := q.GetAllegatoDellaRfq(ctx, db.GetAllegatoDellaRfqParams{AllegatoID: allegato, ThreadID: thread}); err == nil {
		nome = a.NomeFile
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	return traccia.DelGesto(ctx, q, utente, MaxMotivo, WidgetCambioFonte, "la bozza di "+strings.TrimSpace(codiceProdotto)+" passa allo STEP «"+nome+"»")
}

func CambiaSorgenteFormato3(ctx context.Context, pool Avviatore, r CambioFonte3) (VersioneDelServer, error) {
	var out VersioneDelServer
	if !chiaveDelServer(r.Comune) {
		return out, corpo("chiave_salvataggio", "la chiave del salvataggio e' un UUID v4, con l'impronta della richiesta")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, r.ThreadID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return out, err
	}
	if gia, trovata, err := chiaveGiaUsata(ctx, q, r.ThreadID, r.Comune); err != nil || trovata {
		if err != nil {
			return out, err
		}
		return riproponiVersioneDelServer(ctx, q, r.ThreadID, gia)
	}
	prodotto, err := prodottoDellaRfq(ctx, q, r.ThreadID, r.Comune.ProdottoID, true)
	if err != nil {
		return out, err
	}
	b, prima, v, err := bozzaFormato3Bloccata(ctx, q, r.ThreadID, r.Comune)
	if err != nil {
		return out, err
	}
	if r.Proposta == nil {
		return out, corpo("cambia_sorgente", "il cambio della fonte vuole la proposta calcolata dal server")
	}
	vecchio, err := registroDellaVersioneCorrente(ctx, q, r.ThreadID, v)
	if err != nil {
		return out, err
	}
	nuovo := r.Proposta.Registro
	d := r.CambiaSorgente
	// messaggio 46 [U]: senza il motivo dell'utente, la traccia del gesto (la sostituzione della fonte e l'evento del
	// cambio di fonte vogliono un motivo: ck_fonte_motivo, trg_dbv_fonte_motivata)
	if strings.TrimSpace(d.Motivo) == "" {
		if d.Motivo, err = tracciaCambioFonte(ctx, q, r.ThreadID, r.UtenteID, d.Sorgente.AllegatoID, prodotto.Codice); err != nil {
			return out, err
		}
	}
	if nuovo.Contenuto.Proposta.Fonte.AllegatoID != d.Sorgente.AllegatoID.String() {
		a, _ := uuid.Parse(nuovo.Contenuto.Proposta.Fonte.AllegatoID)
		return out, &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoGruppoSuAltraOccorrenza, Campo: "cambia_sorgente.sorgente.gruppo", AllegatoID: &a,
			Messaggio: "il gruppo calcolato sta su un'altra occorrenza dello STEP dichiarato"}
	}
	if d.Sorgente.Sha256 != "" && d.Sorgente.Sha256 != nuovo.Contenuto.Proposta.Fonte.Sha256 {
		return out, corpo("cambia_sorgente.sorgente.sha256", "lo sha256 dichiarato non e' quello dello STEP")
	}
	if err := VerificaImprontaDichiarata("cambia_sorgente.impronta_contenuto", nuovo, d.ImprontaContenuto); err != nil {
		return out, err
	}
	if err := fonteConfermata(ctx, q, r.ThreadID, r.Comune.ProdottoID, nuovo.Contenuto.Proposta.Fonte.Sha256, "cambia_sorgente.sorgente"); err != nil {
		return out, err
	}
	if err := occorrenzeDelRegistro(ctx, q, r.ThreadID, nuovo, "cambia_sorgente"); err != nil {
		return out, err
	}
	in, err := ingressoPartenza(ctx, q, r.ThreadID, r.Comune.ProdottoID)
	if err != nil {
		return out, err
	}
	es, err := Cambia(prima, vecchio, nuovo, d.Scelte, d.Motivo, in)
	if err != nil {
		return out, err
	}
	// direttiva 39 §4 (pbiniziali3.go): le proposte univoche del registro nuovo diventano PB motore (solo con la sorgente
	// confermata da S8), le PB rimaste tengono il loro registro d'origine, gli altri suggerimenti vanno in
	// preassociazioni_proposte[]
	pbIniz, proposte, err := PBInizialiDelServer(ctx, q, r.ThreadID, r.Comune.ProdottoID, &es.Snapshot, &nuovo, nuovo.Contenuto.Proposta.Fonte.Sha256,
		b.BozzaID, int(b.Versione), ricollocabiliDi(prima, es.Vassoio), r.Proposta.step())
	if err != nil {
		return out, err
	}
	es.eventi = append(es.eventi, eventiPBDelServer(pbIniz)...)
	canon, err := es.Snapshot.Canonico()
	if err != nil {
		return out, erroreDellaVersione(err, "cambia_sorgente")
	}
	imp, err := registroDellaVersione(ctx, q, r.ThreadID, r.Comune.ProdottoID, r.UtenteID, nuovo, "cambia_sorgente")
	if err != nil {
		return out, err
	}
	e := es.Effetto
	imp.Differenze = differenzeRaw(e.Differenze)
	imp.FuoriPerCambioFonte, imp.NuoviNodiProposti, imp.DecisioniCadute, imp.CaselleRiscritte, imp.ArchiRiscritti =
		e.FuoriPerCambioFonte, e.NuoviNodiProposti, e.DecisioniCadute, caselleRiscritte3(e.CaselleRiscritte), e.ArchiRiscritti
	algoritmo := AlgoritmoCambioFonte
	if r := es.Riconciliazione; r != nil {
		algoritmo = AlgoritmoCambioFonte2
		imp.CaselleRiscritte = append(imp.CaselleRiscritte, r.Acquisite...)
		imp.AbbinamentiProposti = r.Proposti
	}
	imp.IdentitaAggiornate, imp.DecisioniScritte = es.Aggiornate, es.Scritte
	imp.PreassociazioniIniziali, imp.PreassociazioniProposte = pbIniz, proposte
	for _, x := range e.DaRivedere {
		imp.DaRivedere = append(imp.DaRivedere, DaRivedere3{ChiaveBozza: x.ChiaveBozza, DecisioneID: x.DecisioneID, Motivo: x.Motivo})
	}
	var primaSorgente any
	if prima.Sorgente != nil {
		primaSorgente = prima.Sorgente
	}
	chiaviCaselle := []string{}
	for _, x := range imp.CaselleRiscritte {
		if x.ChiaveBozza != "" {
			chiaviCaselle = append(chiaviCaselle, x.ChiaveBozza)
		} else {
			chiaviCaselle = append(chiaviCaselle, x.DecisioneID)
		}
	}
	dopo := map[string]any{"sorgente": es.Snapshot.Sorgente, "algoritmo": algoritmo,
		"conteggi": map[string]int{"nodi": len(es.Snapshot.Nodi), "archi": len(es.Snapshot.Archi), "fuori_per_cambio_fonte": len(e.FuoriPerCambioFonte),
			"entrati": len(es.Entrati), "nuovi_nodi_proposti": len(e.NuoviNodiProposti), "decisioni_cadute": len(e.DecisioniCadute)},
		"da_rivedere": imp.DaRivedere, "caselle_riscritte": chiaviCaselle, "archi_riscritti": len(e.ArchiRiscritti),
		"identita_aggiornate": chiaviAggiornate(es.Aggiornate), "fuori_per_cambio_fonte": e.FuoriPerCambioFonte,
		"decisioni_cadute": e.DecisioniCadute, "nuovi_nodi_proposti": e.NuoviNodiProposti, "decisioni_scritte": es.Scritte}
	if r := es.Riconciliazione; r != nil {
		dopo["abbinamenti_proposti"] = r.Proposti
		dopo["conteggi"].(map[string]int)["abbinati"] = len(r.Abbinati)
		dopo["conteggi"].(map[string]int)["abbinamenti_proposti"] = len(r.Proposti)
		if len(es.Ricollegati) > 0 {
			dopo["conteggi"].(map[string]int)["registrati_della_fonte_precedente"] = len(es.Ricollegati)
		}
	}
	PresaInCarico{Iniziali: pbIniz, Proposte: proposte}.nelDopo(dopo)
	dopoNeiLimiti(dopo, "nuovi_nodi_proposti", "fuori_per_cambio_fonte", "da_rivedere", "caselle_riscritte", "identita_aggiornate", "decisioni_cadute",
		"decisioni_scritte", "abbinamenti_proposti", "preassociazioni_proposte", "preassociazioni_iniziali")
	finale := eventoServer{Azione: AzSorgenteCambiata, Prima: canonico(primaSorgente), Dopo: canonico(dopo), Motivo: d.Motivo}
	if primaSorgente == nil {
		finale.Prima = nil
	}
	salvato, err := scriviVersioneDelServer(ctx, q, r.ThreadID, r.UtenteID, b, r.Comune, canon, es.Snapshot, imp.ImportazioneID, es.eventi, finale)
	if err != nil {
		return out, err
	}
	// la storia della fonte: sostituzione con la fonte di prima (scelta, se la bozza non ne aveva una STEP)
	fs := db.InsertFonteStepEventoParams{ThreadID: r.ThreadID, ProdottoID: r.Comune.ProdottoID, Ruolo: "prodotto", Azione: "scelta",
		AllegatoID: *es.Snapshot.Sorgente.AllegatoID, Sha256: es.Snapshot.Sorgente.Sha256, Gruppo: testo(es.Snapshot.Sorgente.Gruppo),
		BozzaID: uuid.NullUUID{UUID: b.BozzaID, Valid: true}, Motivo: testo(d.Motivo), AutoreID: r.UtenteID}
	if prima.Sorgente != nil && prima.Sorgente.AllegatoID != nil {
		fs.Azione = "sostituzione"
		fs.PrecedenteAllegatoID = uuid.NullUUID{UUID: *prima.Sorgente.AllegatoID, Valid: true}
		fs.PrecedenteSha256 = testo(prima.Sorgente.Sha256)
		fs.PrecedenteGruppo = testo(prima.Sorgente.Gruppo)
	}
	if _, err := q.InsertFonteStepEvento(ctx, fs); err != nil {
		return out, erroreDiScrittura(err, "cambia_sorgente")
	}
	if r.PrimaDelCommit != nil {
		if err := r.PrimaDelCommit(ctx, q, salvato.BozzaID, salvato.Versione, &imp); err != nil {
			return out, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return out, erroreDelCommit(err, "cambia_sorgente")
	}
	return VersioneDelServer{Salvato3: salvato, Snapshot: canon, Importazione: &imp}, nil
}
