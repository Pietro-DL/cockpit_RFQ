package distintabozza

// R2, forma riallinea_proposta, algoritmo riancoraggio-1 (EM §6.5; U2 §5.2, [U] Q10, [UR] Q10; MAT §4.3 riga 13; direttiva
// 35, blocco C-3): stessa sorgente, proposta nuova. Il gruppo e' il gruppo corrente della sorgente (EM §6.4.1), che chi
// chiama ritrova fra i gruppi di adesso prima della transazione (propostabom.GruppoCorrenteDellaSorgente), con il registro
// gia' confrontato con l'impronta dichiarata (V-7).
//
// La versione n+1 (Riancora):
//   - la sorgente prende il registro nuovo e la sua chiave del gruppo; caselle, archi step e non_includere prendono la
//     chiave nuova, con gli stessi allegato e rif (la voce non cambia, V-3);
//   - un nodo con la casella presente nel registro nuovo (fra le voci o fra i fuori per decisione): proposta con
//     l'identita' proposta cambiata → la scelta recepisci / mantieni (rifonte3.go), da_decidere → l'identita' nuova da
//     sola; una decisione_esistente nuova su un nodo da creare → da_rivedere decisione_esistente (resta crea); un nodo
//     deciso (decisione, registrato, manuale, formato_2), salvo il nodo del prodotto, con la voce cambiata nei campi del
//     contenuto (propostabom.Confronta) → da_rivedere proposta_cambiata; mai sovrascritto;
//   - una decisione identita di un nodo che resta: lettura_assente / lettura_cambiata se la lettura che cita manca o e'
//     diversa nel registro nuovo; mai riscritta;
//   - un nodo con la casella assente dal registro nuovo: la scelta togli (il nodo esce con i suoi archi; le sue PB vanno nel
//     vassoio con il motivo nodo_tolto; le decisioni che lo citano cadono) oppure tieni_senza_casella (la regola per modo
//     del §6.5: proposta → decisione identita alternativa sulla lettura di prima; da_decidere → manuale senza codice;
//     decisione, registrato, formato_2 restano; origine registrato per un componente, utente per un nodo da creare; gli
//     archi step del nodo diventano utente con la stessa qta); senza scelta 422 riallineamento_incompleto;
//   - una non_includere su un rif sparito: la scelta togli, altrimenti riallineamento_incompleto;
//   - le voci nuove non rappresentate non entrano: nuovi_nodi_proposti[]; un arco con la quantita' cambiata resta com'era
//     (differenze.archi[]);
//   - nessuna PA diventa PB ([U] dir. 35 §8 C-3): le preassociazioni restano quelle della bozza, tolte solo con il nodo.
// Gli eventi: i nodo_tolto, arco_tolto, file_svincolato e decisione_tolta delle scelte togli (origine server) e per ultimo
// proposta_riallineata (prima {importazione, gruppo}, dopo {importazione, gruppo, algoritmo, conteggi, da_rivedere[],
// identita_aggiornate[] (chiavi), decisioni_scritte[]}, il motivo): la pila d'annullamento si ferma li'.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// Riallineamento3: una richiesta della forma riallinea_proposta, gia' letta, con la RFQ, l'utente e la proposta calcolata.
type Riallineamento3 struct {
	ThreadID, UtenteID uuid.UUID
	Comune             ComuneR2
	Riallinea          RiallineaR2
	Proposta           *PropostaCalcolata
}

// RiallineaDaRichiesta: la Riallineamento3 di un corpo di R2 nella forma riallinea_proposta.
func (r RichiestaR2) RiallineaDaRichiesta(thread, utente uuid.UUID, p *PropostaCalcolata) Riallineamento3 {
	x := Riallineamento3{ThreadID: thread, UtenteID: utente, Comune: r.Comune, Proposta: p}
	if r.Riallinea != nil {
		x.Riallinea = *r.Riallinea
	}
	return x
}

// VersioneDelServer: la risposta di riallinea_proposta e cambia_sorgente (EM §9.2): la versione n+1 con il suo snapshot e
// il registro con gli elenchi.
type VersioneDelServer struct {
	Salvato3
	Snapshot     json.RawMessage `json:"snapshot"`
	Importazione *ImportazioneR2 `json:"importazione,omitempty"`
}

// EsitoRiancoraggio: la versione n+1 di riancoraggio-1 con i suoi eventi del server e gli elenchi.
type EsitoRiancoraggio struct {
	Snapshot           Snapshot3
	eventi             []eventoServer
	DaRivedere         []DaRivedere3
	IdentitaAggiornate []IdentitaAggiornata3
	DecisioniScritte   []string
	NuoviNodiProposti  []string
	NodiTolti          []string
	NodiTenuti         []string
	Vassoio            []string
	Differenze         propostabom.Differenze
}

// Riancora costruisce la versione n+1 di riancoraggio-1 (commento di testa). prima e' la versione corrente della bozza,
// vecchio il suo registro (nil senza: una bozza da-conferma-1 o convertita), nuovo il registro del gruppo corrente.
func Riancora(prima Snapshot3, vecchio *importazione.Registro, nuovo importazione.Registro, scelte []SceltaR2, motivo, cliente string) (EsitoRiancoraggio, error) {
	var out EsitoRiancoraggio
	if prima.Sorgente == nil || prima.Sorgente.AllegatoID == nil {
		return out, corpoMotivo("riallinea_proposta", MotivoGruppoNonImportabile, "la bozza non ha una sorgente STEP da riallineare")
	}
	allegato := *prima.Sorgente.AllegatoID
	nc := nuovo.Contenuto
	if nc.Proposta.Fonte.AllegatoID != allegato.String() || nc.Proposta.Fonte.Sha256 != prima.Sorgente.Sha256 {
		return out, anteprimaSuperata("riallinea_proposta", nc.ImprontaContenuto, nil,
			"il registro nuovo non e' della sorgente della bozza: il riallineamento e' «stessa sorgente, proposta nuova»")
	}
	gruppo := nc.Gruppo.Chiave
	c := nuovoCantiere(prima, scelte, motivo)
	c.s.Sorgente = &Sorgente3{AllegatoID: &allegato, Sha256: prima.Sorgente.Sha256, Gruppo: gruppo, Importazione: nuovo.Impronta}
	cambiate := map[string]bool{}
	if vecchio != nil {
		conf := propostabom.Confronta(vecchio.Contenuto, nc)
		out.Differenze = conf.Differenze
		for _, d := range conf.Differenze.Nodi {
			if d.Tipo == propostabom.DifferenzaCambiato && d.Voce != "" {
				cambiate[d.Voce] = true
			}
		}
	} else {
		var voci []string
		for _, n := range prima.Nodi {
			for _, x := range n.Caselle {
				voci = append(voci, "n:"+x.AllegatoID.String()+":"+x.Rif)
			}
		}
		out.Differenze = propostabom.ConfrontaCaselle(voci, nc)
	}
	nelNuovo := func(rif string) bool { return VoceDelRegistro(&nc, allegato, rif) }
	vecchiaConDecisione := func(rif string) bool {
		if vecchio == nil {
			return false
		}
		v, ok := vecchio.Contenuto.VocePerRif(allegato, rif)
		_, con := decisioneEsistente(v)
		return ok && con
	}
	chiavi := make([]string, 0, len(prima.Nodi))
	for _, n := range prima.Nodi {
		chiavi = append(chiavi, n.ChiaveBozza)
	}
	for _, k := range chiavi {
		i := indiceNodo(c.s, k)
		n := c.s.Nodi[i]
		if len(n.Caselle) == 0 {
			continue
		}
		rif := n.Caselle[0].Rif
		radice := n.ComponenteID != nil && *n.ComponenteID == prima.ProdottoID
		if !nelNuovo(rif) {
			if radice {
				return out, anteprimaSuperata("riallinea_proposta", nc.ImprontaContenuto, nil, "la radice della bozza non e' nel registro nuovo")
			}
			switch c.scelta(k, "") {
			case SceltaTogli:
				c.togliNodo(k, SvincoloNodoTolto)
				out.NodiTolti = append(out.NodiTolti, k)
			case SceltaTieniSenzaCasella:
				if err := c.tieniSenzaCasella(i, vecchio, allegato, cliente); err != nil {
					return out, err
				}
				out.NodiTenuti = append(out.NodiTenuti, k)
			case "":
				c.manca(k + " (togli o tieni_senza_casella)")
			default:
				c.manca(k + " (la scelta e' togli o tieni_senza_casella)")
			}
			continue
		}
		for j := range c.s.Nodi[i].Caselle {
			c.s.Nodi[i].Caselle[j].Gruppo = gruppo
		}
		if radice {
			continue
		}
		voce := "n:" + allegato.String() + ":" + rif
		switch n.Identita.Modo {
		case ModoProposta, ModoDaDecidere:
			var nuova Identita
			var err error
			if v, ok := nc.VocePerRif(allegato, rif); ok {
				nuova, err = identitaNuovaDellaVoce(v, cliente)
				if _, con := decisioneEsistente(v); con && !vecchiaConDecisione(rif) {
					c.rivedi(k, "", RivedereDecisioneEsistente)
				}
			} else {
				x, _ := nc.FuoriPerRif(allegato, rif)
				nuova, err = identitaDellaVoceFuori(x, cliente)
			}
			if err != nil {
				return out, err
			}
			if err := c.aggiornaIdentita(i, nuova, vecchio, allegato, rif); err != nil {
				return out, err
			}
		default:
			if cambiate[voce] {
				c.rivedi(k, "", RivederePropostaCambiata)
			}
		}
	}
	// le decisioni: identita (la lettura citata), non_includere (il rif sparito)
	for _, d := range append([]Decisione3(nil), c.s.Decisioni...) {
		switch d.Tipo {
		case DecisioneIdentita:
			if indiceNodo(c.s, d.ChiaveBozza) >= 0 {
				if m := motivoLettura(nc, d); m != "" {
					c.rivedi(d.ChiaveBozza, "", m)
				}
			}
		case DecisioneNonIncludere:
			if d.Casella == nil || d.Casella.AllegatoID != allegato {
				continue
			}
			if nelNuovo(d.Casella.Rif) {
				for j := range c.s.Decisioni {
					if c.s.Decisioni[j].ID == d.ID {
						x := *d.Casella
						x.Gruppo = gruppo
						c.s.Decisioni[j].Casella = &x
					}
				}
				continue
			}
			switch c.scelta("", d.ID) {
			case SceltaTogli:
				c.togliDecisione(d.ID)
			default:
				c.manca("decisione " + d.ID + " (togli)")
			}
		}
	}
	for i := range c.s.Archi {
		if c.s.Archi[i].Step != nil {
			st := *c.s.Archi[i].Step
			st.Gruppo = gruppo
			c.s.Archi[i].Step = &st
		}
	}
	if e := c.controllaScelte("riallinea_proposta.scelte", scelte); e != nil {
		return out, e
	}
	out.NuoviNodiProposti = nuoviNodiProposti(c.s, nc, allegato)
	riempiChiavi(&out.Differenze, c.s, allegato)
	c.chiude()
	out.Snapshot, out.eventi = c.s, c.eventi
	out.DaRivedere, out.IdentitaAggiornate, out.DecisioniScritte, out.Vassoio = c.elencoRivedere(), c.aggiornate, c.scritte, c.vassoio
	return out, nil
}

func indiceNodo(s Snapshot3, chiave string) int {
	for i, n := range s.Nodi {
		if n.ChiaveBozza == chiave {
			return i
		}
	}
	return -1
}

// tieniSenzaCasella: «Tieni come nodo a mano», la regola per modo del §6.5 (commento di testa).
func (c *cantiere) tieniSenzaCasella(i int, vecchio *importazione.Registro, allegato uuid.UUID, cliente string) error {
	n := &c.s.Nodi[i]
	rif := n.Caselle[0].Rif
	switch n.Identita.Modo {
	case ModoProposta:
		if err := c.mantieni(i, vecchio, allegato, rif); err != nil {
			return err
		}
	case ModoDaDecidere:
		n.Identita = Identita{Emittente: cliente, EmittentePresunto: true, Modo: ModoManuale}
	}
	n = &c.s.Nodi[i]
	n.Caselle = nil
	if n.Op == OpRiusa || n.Op == OpModifica {
		n.Origine = OrigineRegistrato
	} else {
		n.Origine = OrigineUtente
	}
	for j := range c.s.Archi {
		a := &c.s.Archi[j]
		if (a.Padre == n.ChiaveBozza || a.Figlio == n.ChiaveBozza) && a.Origine == OrigineStep {
			a.Origine, a.Step = OrigineUtente, nil
		}
	}
	return nil
}

// nuoviNodiProposti: le voci del registro (non la radice) che nessun nodo e nessuna non_includere rappresenta.
func nuoviNodiProposti(s Snapshot3, c importazione.Contenuto, allegato uuid.UUID) []string {
	rappresentate := map[string]bool{}
	for _, n := range s.Nodi {
		for _, x := range n.Caselle {
			rappresentate[x.Rif] = true
		}
	}
	for _, d := range s.Decisioni {
		if d.Tipo == DecisioneNonIncludere && d.Casella != nil {
			rappresentate[d.Casella.Rif] = true
		}
	}
	out := []string{}
	for _, v := range c.Proposta.Nodi {
		if c.EVoceDellaRadice(v) || rappresentate[v.Origine.Rif] || v.Origine.AllegatoID != allegato.String() {
			continue
		}
		out = append(out, v.Voce)
	}
	return out
}

// riempiChiavi: la chiave della bozza dei nodi delle differenze che la bozza rappresenta (PE-18: la voce, o il nodo).
func riempiChiavi(d *propostabom.Differenze, s Snapshot3, allegato uuid.UUID) {
	perVoce := map[string]string{}
	for _, n := range s.Nodi {
		for _, x := range n.Caselle {
			perVoce["n:"+allegato.String()+":"+x.Rif] = n.ChiaveBozza
		}
	}
	for i := range d.Nodi {
		if k, ok := perVoce[d.Nodi[i].Voce]; ok && d.Nodi[i].ChiaveBozza == "" {
			d.Nodi[i].ChiaveBozza = k
		}
	}
}

// SorgenteDellaBozza: la sorgente della versione corrente di una bozza aperta formato 3, con la radice (fonte_radice del
// registro della versione, altrimenti il rif della casella del nodo del prodotto): i tre dati con cui si ritrova il
// gruppo corrente (EM §6.4.1). Sola lettura; chi chiama la usa prima del calcolo del motore. ok falso: la bozza non e'
// aperta, non e' del prodotto, non e' formato 3 o non ha una sorgente STEP (lo dira' il servizio sotto il lucchetto).
func SorgenteDellaBozza(ctx context.Context, q *db.Queries, thread, prodotto, bozza uuid.UUID) (propostabom.SorgenteBozza, bool, error) {
	var out propostabom.SorgenteBozza
	b, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: bozza, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if b.ProdottoID != prodotto || b.Stato != StatoAperta || int(b.Formato) != FormatoSnapshot3 {
		return out, false, nil
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: bozza, ThreadID: thread, Versione: b.Versione})
	if err != nil {
		return out, false, err
	}
	s, _, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
	if err != nil {
		return out, false, err
	}
	if s.Sorgente == nil || s.Sorgente.AllegatoID == nil {
		return out, false, nil
	}
	out = propostabom.SorgenteBozza{AllegatoID: *s.Sorgente.AllegatoID, Sha256: s.Sorgente.Sha256, Gruppo: s.Sorgente.Gruppo}
	if v.ImportazioneID.Valid {
		r, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: v.ImportazioneID.UUID, ThreadID: thread})
		if err != nil {
			return out, false, err
		}
		out.Radice = r.FonteRadice.String
	}
	if out.Radice == "" {
		for _, n := range s.Nodi {
			if n.ComponenteID != nil && *n.ComponenteID == prodotto && len(n.Caselle) > 0 {
				out.Radice = n.Caselle[0].Rif
			}
		}
	}
	return out, true, nil
}

// GruppoDelRiallineamento: il gruppo che riallinea_proposta importa, il gruppo corrente della sorgente (EM §6.4.1, §6.5
// passo 1): stessa_occorrenza → la sua chiave; altra_occorrenza → 422 gruppo_su_altra_occorrenza con l'allegato (si segue
// con cambia_sorgente); assente → 422 gruppo_non_importabile.
func GruppoDelRiallineamento(gruppi []propostabom.Gruppo, nonAttivi propostabom.NonAttivi, s propostabom.SorgenteBozza) (string, error) {
	gc := propostabom.GruppoCorrenteDellaSorgente(gruppi, nonAttivi, s)
	switch gc.Come {
	case propostabom.ComeStessaOccorrenza:
		return gc.Chiave, nil
	case propostabom.ComeAltraOccorrenza:
		return "", &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoGruppoSuAltraOccorrenza, Campo: "riallinea_proposta", AllegatoID: gc.AllegatoID,
			Messaggio: "la BOM di lavoro sta su un'altra copia dello stesso file: per seguirla, «Cambia lo STEP della BOM…» (cambia_sorgente)"}
	default:
		return "", corpoMotivo("riallinea_proposta", MotivoGruppoNonImportabile,
			"il Motore A non propone piu' questa struttura per il prodotto (%s): la bozza resta valida", gc.Motivo)
	}
}

// RiallineaFormato3: R2 riallinea_proposta (commento di testa). L'ordine: BloccaThread → la chiave (riproposto,
// CHIAVE_RIUSATA) → il prodotto → la bozza (della RFQ, del prodotto, aperta, alla versione attesa, formato 3) → il
// registro nuovo della stessa sorgente e V-7 → le occorrenze del registro → riancoraggio-1 → il registro trovato o scritto,
// la versione n+1 e gli eventi.
func RiallineaFormato3(ctx context.Context, pool Avviatore, r Riallineamento3) (VersioneDelServer, error) {
	var out VersioneDelServer
	if r.Comune.ChiaveSalvataggio.Version() != 4 || !reSha256.MatchString(r.Comune.ImprontaRichiesta) {
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
	if _, err := prodottoDellaRfq(ctx, q, r.ThreadID, r.Comune.ProdottoID, true); err != nil {
		return out, err
	}
	b, prima, v, err := bozzaFormato3Bloccata(ctx, q, r.ThreadID, r.Comune)
	if err != nil {
		return out, err
	}
	if r.Proposta == nil {
		if prima.Sorgente == nil || prima.Sorgente.AllegatoID == nil {
			return out, corpoMotivo("riallinea_proposta", MotivoGruppoNonImportabile, "la bozza non ha una sorgente STEP da riallineare")
		}
		return out, corpo("riallinea_proposta", "il riallineamento vuole la proposta calcolata dal server")
	}
	vecchio, err := registroDellaVersioneCorrente(ctx, q, r.ThreadID, v)
	if err != nil {
		return out, err
	}
	nuovo := r.Proposta.Registro
	if err := VerificaImprontaDichiarata("riallinea_proposta.impronta_contenuto", nuovo, r.Riallinea.ImprontaContenuto); err != nil {
		return out, err
	}
	if err := occorrenzeDelRegistro(ctx, q, r.ThreadID, nuovo, "riallinea_proposta"); err != nil {
		return out, err
	}
	cliente, err := q.GetClienteDellaRfq(ctx, r.ThreadID)
	if err != nil {
		return out, err
	}
	es, err := Riancora(prima, vecchio, nuovo, r.Riallinea.Scelte, r.Riallinea.Motivo, EmittenteCliente(cliente))
	if err != nil {
		return out, err
	}
	// direttiva 39 §4 (pbiniziali3.go): come cambia_sorgente, sullo stesso STEP
	pbIniz, proposte, err := PBInizialiDelServer(ctx, q, r.ThreadID, r.Comune.ProdottoID, &es.Snapshot, &nuovo, nuovo.Contenuto.Proposta.Fonte.Sha256,
		b.BozzaID, int(b.Versione), ricollocabiliDi(prima, es.Vassoio), r.Proposta.step())
	if err != nil {
		return out, err
	}
	es.eventi = append(es.eventi, eventiPBDelServer(pbIniz)...)
	canon, err := es.Snapshot.Canonico()
	if err != nil {
		return out, erroreDellaVersione(err, "riallinea_proposta")
	}
	imp, err := registroDellaVersione(ctx, q, r.ThreadID, r.Comune.ProdottoID, r.UtenteID, nuovo, "riallinea_proposta")
	if err != nil {
		return out, err
	}
	imp.Differenze = differenzeRaw(es.Differenze)
	imp.NuoviNodiProposti, imp.DaRivedere, imp.IdentitaAggiornate, imp.DecisioniScritte = es.NuoviNodiProposti, es.DaRivedere,
		es.IdentitaAggiornate, es.DecisioniScritte
	imp.PreassociazioniIniziali, imp.PreassociazioniProposte = pbIniz, proposte
	gruppoPrima := ""
	var importazionePrima any
	if prima.Sorgente != nil {
		gruppoPrima = prima.Sorgente.Gruppo
		if prima.Sorgente.Importazione != "" {
			importazionePrima = prima.Sorgente.Importazione
		}
	}
	dopo := map[string]any{"importazione": nuovo.Impronta, "gruppo": nuovo.Contenuto.Gruppo.Chiave, "algoritmo": AlgoritmoRiancoraggio,
		"conteggi": map[string]int{"nodi": len(es.Snapshot.Nodi), "archi": len(es.Snapshot.Archi), "nodi_tolti": len(es.NodiTolti),
			"nodi_tenuti": len(es.NodiTenuti), "nuovi_nodi_proposti": len(es.NuoviNodiProposti)},
		"da_rivedere": es.DaRivedere, "identita_aggiornate": chiaviAggiornate(es.IdentitaAggiornate), "decisioni_scritte": es.DecisioniScritte,
		"nodi_tolti": es.NodiTolti, "nodi_tenuti": es.NodiTenuti}
	PresaInCarico{Iniziali: pbIniz, Proposte: proposte}.nelDopo(dopo)
	dopoNeiLimiti(dopo, "da_rivedere", "nodi_tolti", "nodi_tenuti", "identita_aggiornate", "decisioni_scritte", "preassociazioni_proposte",
		"preassociazioni_iniziali")
	finale := eventoServer{Azione: AzPropostaRiallineata, Prima: canonico(map[string]any{"importazione": importazionePrima, "gruppo": gruppoPrima}),
		Dopo: canonico(dopo), Motivo: r.Riallinea.Motivo}
	salvato, err := scriviVersioneDelServer(ctx, q, r.ThreadID, r.UtenteID, b, r.Comune, canon, es.Snapshot, imp.ImportazioneID, es.eventi, finale)
	if err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, erroreDelCommit(err, "riallinea_proposta")
	}
	return VersioneDelServer{Salvato3: salvato, Snapshot: canon, Importazione: &imp}, nil
}

// bozzaFormato3Bloccata: la bozza aperta formato 3 del corpo, sotto il suo lucchetto, alla versione attesa (della RFQ, del
// prodotto), con lo snapshot della versione corrente.
func bozzaFormato3Bloccata(ctx context.Context, q *db.Queries, thread uuid.UUID, c ComuneR2) (db.DistintaBozza, Snapshot3, db.GetDistintaBozzaVersioneRow, error) {
	var zero db.GetDistintaBozzaVersioneRow
	b, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: c.BozzaID, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return b, Snapshot3{}, zero, fuori("bozza_id", "la bozza %s non e' di questa RFQ", c.BozzaID)
	}
	if err != nil {
		return b, Snapshot3{}, zero, err
	}
	if b.ProdottoID != c.ProdottoID {
		return b, Snapshot3{}, zero, &Errore{Codice: CodiceBozzaAltroProdotto, Campo: "bozza_id", Messaggio: fmt.Sprintf("la bozza %s e' di un altro prodotto", b.BozzaID)}
	}
	if b.Stato != StatoAperta || int(b.Versione) != c.VersioneAttesa {
		return b, Snapshot3{}, zero, conflittoVersione(b, c.VersioneAttesa)
	}
	if int(b.Formato) != FormatoSnapshot3 {
		return b, Snapshot3{}, zero, corpoMotivo("bozza_id", MotivoFormatoDaConvertire, "la bozza e' nel formato %d: prima va convertita (converti_da)", b.Formato)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: thread, Versione: b.Versione})
	if err != nil {
		return b, Snapshot3{}, zero, err
	}
	s, _, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
	if err != nil {
		return b, Snapshot3{}, zero, err
	}
	return b, s, v, nil
}

// registroDellaVersioneCorrente: il registro della versione corrente, riletto con le sue colonne; nil senza registro.
func registroDellaVersioneCorrente(ctx context.Context, q *db.Queries, thread uuid.UUID, v db.GetDistintaBozzaVersioneRow) (*importazione.Registro, error) {
	if !v.ImportazioneID.Valid {
		return nil, nil
	}
	riga, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: v.ImportazioneID.UUID, ThreadID: thread})
	if err != nil {
		return nil, err
	}
	reg, _, err := RileggiImportazione(riga)
	if err != nil {
		return nil, err
	}
	return &reg, nil
}

// scriviVersioneDelServer: la versione n+1 costruita dal server, con il registro, la chiave e l'impronta della richiesta, i
// suoi eventi del server (prima quelli dei nodi e dei file, per ultimo il verbo del server, cosi' la pila d'annullamento si
// ferma dopo tutti: EM §9.2).
func scriviVersioneDelServer(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, b db.DistintaBozza, c ComuneR2, canon []byte, s Snapshot3,
	importazioneID uuid.UUID, eventi []eventoServer, finale eventoServer) (Salvato3, error) {
	impronta := jsonImpronta(canon)
	base := baseDelloSnapshot3(s)
	fonteAll, fonteSha, fonteGruppo := colonneFonte3(s.Sorgente)
	versione := int(b.Versione) + 1
	salvataIl, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{
		BozzaID: b.BozzaID, Versione: int32(versione), Snapshot: canon, Impronta: impronta,
		BaseConfermaID: nullUUID(base.ConfermaID), BaseNumero: int32(base.Numero), BaseContatore: base.Contatore,
		FirmaBase: base.Firma, FonteAllegatoID: fonteAll, FonteSha256: fonteSha, FonteGruppo: fonteGruppo, SalvataDa: utente,
		ImportazioneID: uuid.NullUUID{UUID: importazioneID, Valid: importazioneID != uuid.Nil}, ChiaveSalvataggio: uuid.NullUUID{UUID: c.ChiaveSalvataggio, Valid: true},
		ImprontaRichiesta: testo(c.ImprontaRichiesta),
	})
	if err != nil {
		switch vincolo(err) {
		case "distinta_bozza_versione_pkey":
			return Salvato3{}, &Errore{Codice: CodiceBozzaVersione, Motivo: MotivoVersioneDiversa, Messaggio: "un altro salvataggio e' arrivato prima: rileggi la bozza"}
		case "ux_dbv_chiave_salvataggio":
			return Salvato3{}, &Errore{Codice: CodiceChiaveRiusata, Campo: "chiave_salvataggio", Messaggio: "questa chiave di salvataggio e' gia' usata"}
		}
		return Salvato3{}, erroreDiScrittura(err, "snapshot")
	}
	n0, err := q.UltimoEventoDistintaBozza(ctx, db.UltimoEventoDistintaBozzaParams{BozzaID: b.BozzaID, ThreadID: thread})
	if err != nil {
		return Salvato3{}, err
	}
	tutti := append(append([]eventoServer(nil), eventi...), finale)
	for i, e := range tutti {
		if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: b.BozzaID, N: n0 + int32(i) + 1, Versione: int32(versione),
			Vocabolario: VocabolarioFormato3, Azione: e.Azione, ChiaveBozza: testo(e.ChiaveBozza), Prima: rawONull(e.Prima), Dopo: rawONull(e.Dopo),
			Origine: "server", Motivo: testo(e.Motivo), UtenteID: utente}); err != nil {
			return Salvato3{}, erroreDiScrittura(err, fmt.Sprintf("eventi[%d]", i))
		}
	}
	return Salvato3{BozzaID: b.BozzaID, Formato: FormatoSnapshot3, Versione: versione, NuovaVersione: true, Eventi: len(tutti), Impronta: impronta,
		SalvataIl: salvataIl}, nil
}

// riproponiVersioneDelServer: il reinvio di una forma del server gia' fatta (stessa chiave, stesso corpo): la versione, il
// suo snapshot, il registro e gli elenchi del dopo dell'evento del server (gli elenchi ridotti nell'evento restano il loro
// conteggio [T]).
func riproponiVersioneDelServer(ctx context.Context, q *db.Queries, thread uuid.UUID, v db.GetVersionePerChiaveSalvataggioRow) (VersioneDelServer, error) {
	out := VersioneDelServer{Salvato3: Salvato3{BozzaID: v.BozzaID, Formato: FormatoSnapshot3, Versione: int(v.Versione), Riproposto: true,
		Impronta: strings.TrimSpace(v.Impronta), SalvataIl: v.SalvataIl}}
	r, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: v.BozzaID, ThreadID: thread, Versione: v.Versione})
	if err != nil {
		return out, err
	}
	s, _, err := rileggiSnapshot3(r.Snapshot, r.Impronta)
	if err != nil {
		return out, err
	}
	if out.Snapshot, err = s.codifica(); err != nil {
		return out, err
	}
	if !r.ImportazioneID.Valid {
		return out, nil
	}
	reg, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: r.ImportazioneID.UUID, ThreadID: thread})
	if err != nil {
		return out, err
	}
	t := TestataImportazione(reg)
	imp := &ImportazioneR2{ImportazioneID: t.ImportazioneID, Impronta: t.Impronta, ImprontaContenuto: t.ImprontaContenuto, ImprontaEvidenze: t.ImprontaEvidenze,
		Riusata: true}
	ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: v.BozzaID, ThreadID: thread, VersioneMax: v.Versione, DopoN: 0, Limite: 1 << 20})
	if err == nil {
		for _, e := range ee {
			if e.Versione != v.Versione || e.Dopo == nil || (e.Azione != AzPropostaRiallineata && e.Azione != AzSorgenteCambiata) {
				continue
			}
			var d struct {
				DaRivedere         []DaRivedere3              `json:"da_rivedere"`
				DecisioniScritte   []string                   `json:"decisioni_scritte"`
				FuoriPerCambio     []string                   `json:"fuori_per_cambio_fonte"`
				DecisioniCadute    []string                   `json:"decisioni_cadute"`
				NuoviNodiProposti  []string                   `json:"nuovi_nodi_proposti"`
				IdentitaAggiornate []string                   `json:"identita_aggiornate"`
				Iniziali           []Preassociazione3         `json:"preassociazioni_iniziali"`
				Proposte           []PreassociazioneProposta3 `json:"preassociazioni_proposte"`
			}
			if json.Unmarshal(*e.Dopo, &d) == nil {
				imp.DaRivedere, imp.DecisioniScritte, imp.FuoriPerCambioFonte, imp.DecisioniCadute, imp.NuoviNodiProposti =
					d.DaRivedere, d.DecisioniScritte, d.FuoriPerCambio, d.DecisioniCadute, d.NuoviNodiProposti
				imp.PreassociazioniIniziali, imp.PreassociazioniProposte = d.Iniziali, d.Proposte
				for _, k := range d.IdentitaAggiornate {
					imp.IdentitaAggiornate = append(imp.IdentitaAggiornate, IdentitaAggiornata3{ChiaveBozza: k})
				}
			}
		}
	}
	out.Importazione = imp
	return out, nil
}

// differenzeRaw: le differenze nella forma di PE-18, assenti se vuote (B-9).
func differenzeRaw(d propostabom.Differenze) json.RawMessage {
	if d.IsZero() {
		return nil
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil
	}
	return b
}

// erroreDellaVersione: uno snapshot costruito dal server che non passa la forma del formato 3 e' un rifiuto della forma, mai
// un 5xx.
func erroreDellaVersione(err error, campo string) error {
	var e *Errore
	if errors.As(err, &e) {
		x := *e
		x.Messaggio = fmt.Sprintf("la versione costruita dal server non ha la forma del formato 3 (%s): %s", e.Campo, e.Messaggio)
		x.Campo = campo
		return &x
	}
	return err
}

// erroreDelCommit: i rifiuti dei trigger al COMMIT di una forma del server sono un 422 sul campo della forma.
func erroreDelCommit(err error, campo string) error {
	if s := sqlstate(err); s == "P4X04" || s == "P4X07" || s == "P4X08" {
		return corpo(campo, "il database rifiuta la versione (%s)", s)
	}
	return err
}
