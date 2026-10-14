package ancoraggio

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// Le equivalenze del cartiglio (K4, opzione A dell'utente): un cartiglio che scrive il codice della parte (A:M, per
// esempio il materiale) e, nella stessa entità, il documento 3D della stessa parte (cartiglio.riferimento_3d, B:D3:
// il Ref.3D, che è l'id del nodo dello STEP) dice che M e D3 sono la stessa parte. La regola è generica: si accende solo
// quando una grammatica legge cartiglio.riferimento_3d (il router ne fa una relazione, mai un'identità del file), e i
// namespace della coppia vengono dalle letture, non da una configurazione. L'equivalenza non diventa mai un'identità
// del file, né un namespace che si confronta (la guardia L8 resta com'è: la tabella è parziale): dà soltanto un
// candidato proposto a livello componente, con il motivo equivalenza_cartiglio e le sue prove (ancora, secondo giro).

// campoCartiglioRiferimento3D: il campo del cartiglio con il documento 3D della stessa parte (evidenze, K4).
const campoCartiglioRiferimento3D = "riferimento_3d"

// Le fonti di una prova di equivalenza (ProvaEquivalenza.Fonte): il codice della parte letto nel cartiglio, accanto al
// riferimento 3D, o, quando il cartiglio non ne legge uno, il nome dello stesso file.
const (
	FonteEquivalenzaCartiglio = "cartiglio"
	FonteEquivalenzaNomeFile  = "nome_file"
)

// ProvaEquivalenza: da dove viene un'equivalenza del cartiglio (K4): l'allegato, l'entità del cartiglio che porta il
// riferimento 3D, la lettura del codice della parte (A:M) e la lettura del riferimento (B:D3), la fonte della lettura
// del codice (cartiglio | nome_file). Due gemelli (il PDF e il DXF dello stesso disegno) sono una coppia sola con due
// prove.
type ProvaEquivalenza struct {
	AllegatoID         uuid.UUID `json:"allegato_id"`
	Entita             string    `json:"entita"`
	LetturaCodice      string    `json:"lettura_codice"`
	LetturaRiferimento string    `json:"lettura_riferimento"`
	Fonte              string    `json:"fonte"`
}

// equivalenza: una coppia del thread, dal codice della parte (A:M) al riferimento 3D (B:D3), con la forma del
// riferimento (la prima lettura in ordine di allegato e di ID) e le prove in ordine.
type equivalenza struct {
	codice, riferimento string // le chiavi (chiaveEquivalenza)
	forma               motorea.LetturaForma
	formaCodice         motorea.LetturaForma // la forma del codice della parte, dalla stessa prima lettura
	prove               []ProvaEquivalenza
}

// CodiceEquivalente: il codice della parte che i cartigli del thread scrivono accanto al documento 3D che è l'identità
// del nodo (K4, CatenaCodice.Equivalente; collaudo dell'utente del 14/10: il nodo dello STEP si chiama col Ref.3D, ma
// la parte è il P/N del cartiglio). È una proposta, come l'equivalenza da cui viene: non cambia l'identità del nodo
// (che resta quella dello STEP), non è una decisione e non entra nei confronti dei namespace. Namespace, Base
// (normalizzata), Marcatore: del codice della parte; Prove: quelle dell'equivalenza, in ordine.
type CodiceEquivalente struct {
	Namespace string             `json:"namespace"`
	Base      string             `json:"base"`
	Marcatore string             `json:"marcatore,omitempty"`
	Prove     []ProvaEquivalenza `json:"prove"`
}

// Fonte: FonteEquivalenzaCartiglio se almeno una prova legge il codice della parte nel cartiglio, altrimenti
// FonteEquivalenzaNomeFile (il codice viene solo dal nome dei file il cui cartiglio scrive il riferimento 3D). Serve
// alla pagina, che deve dire da dove viene il P/N.
func (c *CodiceEquivalente) Fonte() string {
	for _, p := range c.Prove {
		if p.Fonte == FonteEquivalenzaCartiglio {
			return FonteEquivalenzaCartiglio
		}
	}
	return FonteEquivalenzaNomeFile
}

// Disegni: quanti file distinti portano le prove (il PDF e il DXF dello stesso disegno sono due file).
func (c *CodiceEquivalente) Disegni() int {
	visti := map[uuid.UUID]bool{}
	for _, p := range c.Prove {
		visti[p.AllegatoID] = true
	}
	return len(visti)
}

// equivalenzeSuiNodi: per ogni nodo delle strutture con una lettura scelta e la base completa, il codice della parte
// dell'unica equivalenza del thread il cui riferimento 3D è l'identità del nodo (stesso namespace, base uguale o
// equivalente, marcatori compatibili: le regole del secondo giro di ancora). Con più equivalenze possibili, nessuna:
// a parità non si sceglie.
func equivalenzeSuiNodi(strutture []StrutturaProdotto, eq map[string]*equivalenza) {
	if len(eq) == 0 {
		return
	}
	chiavi := make([]string, 0, len(eq))
	for k := range eq {
		chiavi = append(chiavi, k)
	}
	sort.Strings(chiavi)
	for si := range strutture {
		for ni := range strutture[si].Nodi {
			c := &strutture[si].Nodi[ni].Codice
			f := c.Forma
			if f == nil || !baseCompleta(*f) {
				continue
			}
			var scelta *equivalenza
			piu := false
			for _, k := range chiavi {
				e := eq[k]
				if e.forma.Namespace != f.Namespace || !marcatoriCompatibili(marcatoreDi(e.forma), marcatoreDi(*f)) {
					continue
				}
				if v := motorea.ConfrontaBasi(f.Base, e.forma.Base); v != motorea.CompatibilitaUguale && v != motorea.CompatibilitaEquivalente {
					continue
				}
				if scelta != nil && scelta != e {
					piu = true
				}
				scelta = e
			}
			if scelta == nil || piu {
				continue
			}
			c.Equivalente = &CodiceEquivalente{Namespace: scelta.formaCodice.Namespace, Base: scelta.formaCodice.Base.Normalizzata,
				Marcatore: marcatoreDi(scelta.formaCodice), Prove: append([]ProvaEquivalenza(nil), scelta.prove...)}
		}
	}
}

// chiaveEquivalenza: la chiave di un lato della coppia, namespace, base normalizzata e marcatore scritto, con la regola
// unica dell'identità (T-B4-30): due marcatori scritti e diversi sono due chiavi, e quindi una contesa.
func chiaveEquivalenza(f motorea.LetturaForma) string {
	return f.Namespace + "\x00" + f.Base.Normalizzata + "\x00" + marcatoreDi(f)
}

// rifEquivalenza: il riferimento di una chiave per le diagnostiche (RifIdentita, senza revisione).
func rifEquivalenza(k string) string {
	p := strings.SplitN(k, "\x00", 3)
	for len(p) < 3 {
		p = append(p, "")
	}
	return RifIdentita(p[0], p[1], p[2], "")
}

// baseCompleta: la lettura ha la base completa (nessun segmento mancante, nessuna ripetizione che non concorda): solo
// così una base sta in un'equivalenza.
func baseCompleta(f motorea.LetturaForma) bool {
	if !f.Base.Completa || len(f.Base.Mancanti) > 0 {
		return false
	}
	for _, r := range f.Ripetizioni {
		if !r.Concorda {
			return false
		}
	}
	return true
}

// equivalenzeDelThread: la tabella delle equivalenze del cartiglio dei file del thread (già in ordine di allegato), per
// chiave del codice della parte, e le diagnostiche delle contese. Per ogni file e ogni entità del cartiglio (in ordine):
//   - i riferimenti: le letture su cartiglio.riferimento_3d dell'entità; senza, niente;
//   - i codici: le letture d'identità su cartiglio.codice della stessa entità (fonte cartiglio); solo se l'entità non
//     ne ha, le letture d'identità del nome del file (fonte nome_file) fuori dai namespace dei riferimenti
//     (nomeFuoriDaiRiferimenti);
//   - basi tutte complete, una chiave sola per lato, namespace diversi; con più chiavi in un lato letto nel
//     cartiglio, nessuna coppia e la diagnostica; con più chiavi nel nome del file, una base non completa o lo stesso
//     namespace, nessuna coppia (non è una contesa).
//
// Poi, a livello di thread: uno stesso riferimento con due codici, o uno stesso codice con due riferimenti, toglie
// tutte le coppie coinvolte, con una diagnostica per chiave contesa; e così una coppia che ha una chiave in un cartiglio
// conteso in sé (più codici o più riferimenti nella stessa entità), con la sua diagnostica. Le mappe servono solo a
// cercare.
func equivalenzeDelThread(files []FileInterpretato) (map[string]*equivalenza, []evidenze.Diagnostica) {
	var diag []evidenze.Diagnostica
	coppie := map[string]*equivalenza{} // codice \x01 riferimento
	var ordine []string
	// le chiavi di un cartiglio conteso in sé (più codici o più riferimenti nella stessa entità): toglie anche le coppie
	// di altri cartigli con le stesse chiavi (revisione del 14/10: senza, un secondo cartiglio coerente avrebbe dato
	// l'equivalenza che il primo contraddice)
	contesoInEntita := map[string]bool{}
	for _, f := range files {
		unita := make(map[string]evidenze.UnitaEvidenza, len(f.Documento.Unita))
		for _, u := range f.Documento.Unita {
			unita[u.ID] = u
		}
		letture := append([]motorea.LetturaCodice(nil), f.Interpretazione.Letture...)
		sort.SliceStable(letture, func(i, j int) bool { return letture[i].ID < letture[j].ID })
		codici, riferimenti := map[string][]motorea.LetturaCodice{}, map[string][]motorea.LetturaCodice{}
		var nome []motorea.LetturaCodice
		var entita []string
		for _, l := range letture {
			u, ok := unita[l.UnitaID]
			if !ok {
				continue
			}
			switch s := u.Selettore; {
			case s.Contesto == evidenze.ContestoNomeFile && l.Funzione == motorea.FunzIdentitaFile:
				nome = append(nome, l)
			case s.Contesto == evidenze.ContestoCartiglio && u.EntitaID != "" && s.Campo.Valore == campoCartiglioCodice && l.Funzione == motorea.FunzIdentitaFile:
				codici[u.EntitaID] = append(codici[u.EntitaID], l)
			case s.Contesto == evidenze.ContestoCartiglio && u.EntitaID != "" && s.Campo.Valore == campoCartiglioRiferimento3D:
				if len(riferimenti[u.EntitaID]) == 0 {
					entita = append(entita, u.EntitaID)
				}
				riferimenti[u.EntitaID] = append(riferimenti[u.EntitaID], l)
			}
		}
		sort.Strings(entita)
		for _, e := range entita {
			rif := riferimenti[e]
			cod, fonte := codici[e], FonteEquivalenzaCartiglio
			if len(cod) == 0 {
				cod, fonte = nomeFuoriDaiRiferimenti(nome, rif), FonteEquivalenzaNomeFile
			}
			if len(cod) == 0 {
				continue
			}
			kc, kr, completa := chiaviDi(cod), chiaviDi(rif), true
			for _, l := range append(append([]motorea.LetturaCodice(nil), cod...), rif...) {
				completa = completa && baseCompleta(l.Forma)
			}
			if !completa {
				continue
			}
			if fonte == FonteEquivalenzaNomeFile && len(kc) != 1 {
				// Più identità nel nome del file (il materiale e il 2D, «210000080_D42_100000001_000_00.dxf», o due
				// materiali) non sono una contesa del cartiglio: il nome dice di più, e il ripiego non sceglie.
				continue
			}
			if len(kc) != 1 || len(kr) != 1 {
				r := []string{idFonteAllegato(f.AllegatoID)}
				for _, k := range append(append([]string(nil), kc...), kr...) {
					r = append(r, rifEquivalenza(k))
				}
				diag = append(diag, diagnosticaContesa(fmt.Sprintf("il cartiglio %q dà %d codici della parte (fonte %s) e %d riferimenti 3D: l'equivalenza vuole uno e uno, e non si propone",
					e, len(kc), fonte, len(kr)), r))
				for _, k := range kc {
					contesoInEntita["codice\x02"+k] = true
				}
				for _, k := range kr {
					contesoInEntita["riferimento\x02"+k] = true
				}
				continue
			}
			if cod[0].Forma.Namespace == rif[0].Forma.Namespace {
				continue
			}
			k := kc[0] + "\x01" + kr[0]
			c, ok := coppie[k]
			if !ok {
				c = &equivalenza{codice: kc[0], riferimento: kr[0], forma: rif[0].Forma, formaCodice: cod[0].Forma}
				coppie[k] = c
				ordine = append(ordine, k)
			}
			for _, lc := range cod {
				for _, lr := range rif {
					c.prove = append(c.prove, ProvaEquivalenza{AllegatoID: f.AllegatoID, Entita: e, LetturaCodice: lc.ID, LetturaRiferimento: lr.ID, Fonte: fonte})
				}
			}
		}
	}

	// Le contese del thread: un codice con più riferimenti, un riferimento con più codici.
	perCodice, perRiferimento := map[string][]string{}, map[string][]string{}
	for _, k := range ordine {
		c := coppie[k]
		perCodice[c.codice] = append(perCodice[c.codice], c.riferimento)
		perRiferimento[c.riferimento] = append(perRiferimento[c.riferimento], c.codice)
	}
	conteso := map[string]bool{}
	var contese []string
	for _, x := range []struct {
		per  map[string][]string
		lato string
	}{{perCodice, "codice"}, {perRiferimento, "riferimento"}} {
		for k, altri := range x.per {
			if len(altri) > 1 {
				conteso[x.lato+"\x02"+k] = true
				contese = append(contese, x.lato+"\x02"+k)
			}
		}
	}
	sort.Strings(contese)
	for _, ck := range contese {
		lato, k, _ := strings.Cut(ck, "\x02")
		altri := perCodice[k]
		cosa := "un codice della parte è scritto nei cartigli del thread accanto a %d riferimenti 3D diversi"
		if lato == "riferimento" {
			altri, cosa = perRiferimento[k], "un riferimento 3D è scritto nei cartigli del thread accanto a %d codici della parte diversi"
		}
		altri = unici(altri)
		r := []string{rifEquivalenza(k)}
		var allegati []string
		for _, a := range altri {
			r = append(r, rifEquivalenza(a))
			kk := k + "\x01" + a
			if lato == "riferimento" {
				kk = a + "\x01" + k
			}
			for _, p := range coppie[kk].prove {
				allegati = append(allegati, idFonteAllegato(p.AllegatoID))
			}
		}
		r = append(r, unici(allegati)...)
		diag = append(diag, diagnosticaContesa(fmt.Sprintf(cosa+" (i Rif): nessuna equivalenza del cartiglio si propone per le coppie coinvolte", len(altri)), r))
	}

	out := map[string]*equivalenza{}
	for _, k := range ordine {
		c := coppie[k]
		if conteso["codice\x02"+c.codice] || conteso["riferimento\x02"+c.riferimento] {
			continue
		}
		if contesoInEntita["codice\x02"+c.codice] || contesoInEntita["riferimento\x02"+c.riferimento] {
			r := []string{rifEquivalenza(c.codice), rifEquivalenza(c.riferimento)}
			var allegati []string
			for _, p := range c.prove {
				allegati = append(allegati, idFonteAllegato(p.AllegatoID))
			}
			r = append(r, unici(allegati)...)
			diag = append(diag, diagnosticaContesa("un codice della parte e un riferimento 3D scritti insieme (i Rif) hanno una chiave in un cartiglio conteso del thread: l'equivalenza non si propone", r))
			continue
		}
		sort.SliceStable(c.prove, func(i, j int) bool { return chiaveProva(c.prove[i]) < chiaveProva(c.prove[j]) })
		var prove []ProvaEquivalenza
		for i, p := range c.prove {
			if i > 0 && chiaveProva(p) == chiaveProva(c.prove[i-1]) {
				continue
			}
			prove = append(prove, p)
		}
		c.prove = prove
		out[c.codice] = c
	}
	return out, diag
}

// nomeFuoriDaiRiferimenti: le letture d'identità del nome del file per il ripiego, senza quelle nel namespace di un
// riferimento 3D dell'entità. Il nome di un 2D vero dice più identità in namespace diversi (il materiale e il
// documento 2D, «210000080_D42_100000001_000_00.dxf»): il documento 2D sta nel namespace del riferimento, e una
// coppia dello stesso namespace non sarebbe comunque un'equivalenza; resta il codice della parte.
func nomeFuoriDaiRiferimenti(nome, rif []motorea.LetturaCodice) []motorea.LetturaCodice {
	ns := map[string]bool{}
	for _, l := range rif {
		ns[l.Forma.Namespace] = true
	}
	var out []motorea.LetturaCodice
	for _, l := range nome {
		if !ns[l.Forma.Namespace] {
			out = append(out, l)
		}
	}
	return out
}

// chiaviDi: le chiavi distinte delle letture, in ordine.
func chiaviDi(l []motorea.LetturaCodice) []string {
	var k []string
	for _, x := range l {
		k = append(k, chiaveEquivalenza(x.Forma))
	}
	return unici(k)
}

func chiaveProva(p ProvaEquivalenza) string {
	return p.AllegatoID.String() + "\x00" + p.Entita + "\x00" + p.LetturaCodice + "\x00" + p.LetturaRiferimento + "\x00" + p.Fonte
}

// diagnosticaContesa: l'avviso di un'equivalenza del cartiglio che non si propone (CodiceEquivalenzaCartiglioContesa).
func diagnosticaContesa(messaggio string, rif []string) evidenze.Diagnostica {
	return evidenze.Diagnostica{Codice: CodiceEquivalenzaCartiglioContesa, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
		Percorso: "file", Messaggio: messaggio, Rif: rif}
}

// ---- il secondo giro di ancora ----

// sostegniPerEquivalenza: per un'identità raggiungibile senza un candidato diretto del file, le letture del file con il
// ruolo componente e la base completa la cui chiave ha un'equivalenza verso un riferimento dello stesso namespace
// dell'identità, con la base uguale o equivalente (ConfrontaBasi). Per la regola della contesa un riferimento ha un solo
// codice, quindi un'identità ha al più un'equivalenza.
func (a *ancoratore) sostegniPerEquivalenza(g *identitaRaggiungibile, ident []motorea.LetturaCodice) ([]sostegno, *equivalenza) {
	var sost []sostegno
	var eq *equivalenza
	for _, l := range ident {
		if !haRuolo(l.RuoliCandidati, grammatica.RuoloComponente) || !baseCompleta(l.Forma) {
			continue
		}
		e, ok := a.equivalenze[chiaveEquivalenza(l.Forma)]
		if !ok || e.forma.Namespace != g.namespace || (eq != nil && e != eq) {
			continue
		}
		if v := motorea.ConfrontaBasi(g.baseLetta, e.forma.Base); v != motorea.CompatibilitaUguale && v != motorea.CompatibilitaEquivalente {
			continue
		}
		sost = append(sost, sostegno{l: l, base: l.Forma.Base})
		eq = e
	}
	return sost, eq
}

// candidatoEquivalenza: il candidato a livello componente su un'identità raggiungibile per un'equivalenza del cartiglio
// (K4): le posizioni, i percorsi, gli archi, l'autorità e il Target come candidatoComponente, il motivo
// equivalenza_cartiglio e le prove. Le dimensioni misurano il RIFERIMENTO contro i nodi, non la lettura del file, che
// è di un altro namespace: namespace non determinabile, base, marcatore e qualificatori del riferimento contro il nodo,
// revisione non determinabile (la revisione del 2D o del materiale non è quella del 3D), la compatibilità con le
// decisioni del nodo sul riferimento. Letture: le letture del file che lo sostengono.
func (a *ancoratore) candidatoEquivalenza(g *identitaRaggiungibile, sost []sostegno, eq *equivalenza) CandidatoAncoraggio {
	c, perDim := a.candidatoSuIdentita(g, MotivoCandidatoEquivalenzaCartiglio)
	r := eq.forma
	for _, x := range sost {
		f := x.l.Forma
		c.Letture = append(c.Letture, x.l.ID)
		for _, nr := range perDim {
			n := a.strutture[nr.s].Nodi[nr.n]
			nf := nr.l.Forma
			c.Dimensioni = append(c.Dimensioni,
				DimensioneCompatibilita{Dimensione: DimensioneNamespace, Esito: confrontaNamespace(nf.Namespace, f.Namespace), Lettura: x.l.ID, Rif: n.Rif},
				DimensioneCompatibilita{Dimensione: DimensioneBase, Esito: motorea.ConfrontaBasi(nf.Base, r.Base), Lettura: x.l.ID, Rif: n.Rif},
				DimensioneCompatibilita{Dimensione: DimensioneMarcatore, Esito: compatibilitaMarcatori(marcatoreDi(nf), marcatoreDi(r)), Lettura: x.l.ID, Rif: n.Rif},
				DimensioneCompatibilita{Dimensione: DimensioneRevisione, Esito: motorea.CompatibilitaNonDeterminabile, Lettura: x.l.ID, Rif: n.Rif},
				DimensioneCompatibilita{Dimensione: DimensioneQualificatori, Esito: confrontaQualificatori(nf.Affissi, r.Affissi), Lettura: x.l.ID, Rif: n.Rif})
			if n.Decisione != nil {
				c.Dimensioni = append(c.Dimensioni, DimensioneCompatibilita{Dimensione: DimensioneCodiceConfermato,
					Esito: compatibilitaConDecisione(r, r.Base, n.Decisione.Lettura, n.Codice.Confermato), Lettura: x.l.ID, Rif: RifComponente(n.Decisione.ComponenteID)})
			}
			if n.RigaLegacy != nil && n.RigaLegacy.CodiceManuale != nil {
				c.Dimensioni = append(c.Dimensioni, DimensioneCompatibilita{Dimensione: DimensioneCodiceManuale,
					Esito: compatibilitaConDecisione(r, r.Base, n.RigaLegacy.LetturaManuale, n.Codice.Manuale), Lettura: x.l.ID, Rif: RifRigaProposta(n.RigaLegacy.ID)})
			}
		}
	}
	c.Equivalenze = append([]ProvaEquivalenza(nil), eq.prove...)
	chiudiCandidato(&c)
	return c
}
