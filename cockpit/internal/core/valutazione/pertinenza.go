package valutazione

import (
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// pertinenza.go: la pertinenza di un file a un prodotto, gli orfani e «da smistare» (R93 [U][R], R105 [U], R91; contratto
// §1.5, il blocco «La pertinenza e gli orfani»; E1R §6.4, §6.5; T-E1-11, T-E1R-11; fase V2 di B6). La pertinenza è per
// evidenza: un file conta nel controllo (il perimetro di R93 b: associazioni.go) ed è pertinente a P se un'evidenza lo
// lega a P; il contesto del messaggio vale solo per un file senza nessun'altra evidenza di pertinenza. Un orfano conta,
// non è terminale e non è pertinente a nessun prodotto: sta in «da smistare», fa un avviso nel fascicolo e non blocca
// niente. Il contesto non è mai un'associazione (non è un candidato, non soddisfa una voce, non conferma niente), e lo
// scarto è solo un gesto dell'utente (R105): nessuna regola qui rende scartato un file.
//
// Le tre domande di B6 sulla pertinenza e su «da smistare» hanno avuto la risposta dell'utente il 07/10
// (domande-a1c.md), e ognuna resta in una funzione sola: contestoApplicabile (R106 B, precisata),
// prodottiNominati (R107, precisata; non coincide con una lettera), voceDaSmistare (R108 A, confermata con il requisito
// operativo per lo spazio di verifica). Dalle precisazioni di R106 e R107 il contesto si conserva, con la sua provenienza,
// per ogni file che conta (AssociazioneFile.Contesto), e la contraddizione con le evidenze o con la decisione resta
// visibile (valutazione.contesto_discorde), senza cambiare la regola della pertinenza.

// I valori della destinazione F8 che la pertinenza guarda (dettagli.destinazione.candidati[].chiave, il vocabolario dello
// Smistamento legacy, C4-08: «componente:<uuid>», «nodo:<proposta_id>», «identificativo:<CODICE>», «generale»), ripetuti
// qui perché il pacchetto che li scrive (core/rfq/fascicolo) non si importa. Se cambiano, lo dicono le prove.
const (
	chiaveF8Componente     = "componente:"
	chiaveF8Nodo           = "nodo:"
	chiaveF8Identificativo = "identificativo:"
	chiaveF8Generale       = "generale"
)

// evidenzeDiPertinenza: i prodotti target a cui il file è pertinente per un'evidenza che non è il contesto del messaggio
// (contratto §1.5; T-E1-11), in ordine, senza doppioni. Solo per un file che conta nel controllo (R93 b):
//   - un candidato del motore A su una struttura di P (candidata o BOM di lavoro), cioè l'associazione candidato_unico,
//     ambiguo o discordante con un candidato in P: le radici raggiungibili del candidato e i target delle sue posizioni
//     (prodottiDelCandidato). Una posizione su un nodo scartato resta un'evidenza: lo scarto del nodo non è uno scarto
//     del file (T-B4-27, R105);
//   - la collocazione non_determinabile per il target P: nessun candidato, il codice del file ha la base di P (una
//     lettura d'identità che nomina P: nomina) e P non ha nessuna struttura;
//   - la destinazione F8 (ogni chiave della destinazione: prodottiDellaDestinazione), un «assegna» o un documento
//     confermato su un componente del perimetro di P (il componente del prodotto e i componenti attivi raggiungibili per
//     le relazioni confermate: R62 e A);
//   - un file che è uno STEP la cui radice ha la base di P: un candidato della fonte di P con l'origine motore_a (R76 b).
func (s *smistamentoThread) evidenzeDiPertinenza(x *fileDelThread) []string {
	if !x.conta() {
		return nil
	}
	var out []string
	if x.af != nil {
		switch x.af.Associazione {
		case ancoraggio.AssociazioneCandidatoUnico, ancoraggio.AssociazioneAmbiguo, ancoraggio.AssociazioneDiscordante:
			for _, c := range x.af.Candidati {
				out = append(out, s.prodottiDelCandidato(c)...)
			}
		}
		if x.af.Collocazione == ancoraggio.CollocazioneNonDeterminabile && len(x.af.Candidati) == 0 && x.letto != nil {
			letture := lettureDiIdentita(*x.letto, *x.af)
			for i, tg := range s.targets {
				if s.prodotti[i].Struttura != ancoraggio.StatoStrutturaNessuna {
					continue
				}
				for _, l := range letture {
					if nomina(tg, l.Forma) {
						out = append(out, tg.pv.Rif)
						break
					}
				}
			}
		}
	}
	for _, k := range x.destinazione {
		out = append(out, s.prodottiDellaDestinazione(k)...)
	}
	if x.manuale != nil {
		out = append(out, s.prodottiDelComponente(*x.manuale)...)
	}
	if x.documento != nil && x.documento.ComponenteID != nil {
		out = append(out, s.prodottiDelComponente(*x.documento.ComponenteID)...)
	}
	for _, pv := range s.prodotti {
		for _, c := range pv.Fonte.Candidati {
			if c.Origine == ancoraggio.CandidatoDaMotoreA && c.AllegatoID != nil && *c.AllegatoID == x.a.ID {
				out = append(out, pv.Rif)
			}
		}
	}
	return ordinatiUnici(out)
}

// prodottiDelCandidato: i prodotti target di un candidato del motore A: il target del livello prodotto, le radici
// raggiungibili (i target da cui il nodo si raggiunge: FIGLIO-CONDIVISO) e i target delle posizioni, che sono target.
func (s *smistamentoThread) prodottiDelCandidato(c ancoraggio.CandidatoAncoraggio) []string {
	var out []string
	if c.Livello == ancoraggio.LivelloProdotto && s.target[c.Target] {
		out = append(out, c.Target)
	}
	for _, r := range c.RadiciRaggiungibili {
		if s.target[r] {
			out = append(out, r)
		}
	}
	for _, p := range c.Posizioni {
		if s.target[p.Target] {
			out = append(out, p.Target)
		}
	}
	return out
}

// prodottiDelComponente: i prodotti target con il componente nel perimetro (la BOM confermata: R62 e A).
func (s *smistamentoThread) prodottiDelComponente(k uuid.UUID) []string {
	var out []string
	for i, pv := range s.prodotti {
		if s.perimetri[i][k] {
			out = append(out, pv.Rif)
		}
	}
	return out
}

// prodottiDellaDestinazione: i prodotti target di una chiave della destinazione F8 (C4-08):
//   - «componente:<uuid>»: i prodotti con quel componente nel perimetro;
//   - «nodo:<proposta_id>»: la riga di componente_proposta, cioè il nodo (sha256, chiave) del suo STEP: i prodotti con quel
//     nodo in una delle loro strutture, più quelli con il componente della riga, se è decisa, nel perimetro;
//   - «identificativo:<CODICE>»: il target con quel codice (la chiave del codice: maiuscole e spazi ai bordi, come i
//     target);
//   - «generale», o una chiave che non si conosce: nessun prodotto.
func (s *smistamentoThread) prodottiDellaDestinazione(chiave string) []string {
	var out []string
	switch {
	case strings.HasPrefix(chiave, chiaveF8Componente):
		if k, err := uuid.Parse(strings.TrimPrefix(chiave, chiaveF8Componente)); err == nil {
			out = append(out, s.prodottiDelComponente(k)...)
		}
	case strings.HasPrefix(chiave, chiaveF8Nodo):
		id, err := uuid.Parse(strings.TrimPrefix(chiave, chiaveF8Nodo))
		if err != nil {
			break
		}
		r := s.righe[id]
		if r == nil {
			break
		}
		if r.Sha256 != "" && r.Chiave != "" {
			nodo := ancoraggio.RifNodo(r.Sha256, r.Chiave)
			for i, pv := range s.prodotti {
				if s.nodi[i][nodo] {
					out = append(out, pv.Rif)
				}
			}
		}
		if r.ComponenteID != nil && (r.Stato == statoPropostaConfermata || r.Stato == statoPropostaDuplicato) {
			out = append(out, s.prodottiDelComponente(*r.ComponenteID)...)
		}
	case strings.HasPrefix(chiave, chiaveF8Identificativo):
		codice := chiaveCodice(strings.TrimPrefix(chiave, chiaveF8Identificativo))
		for _, pv := range s.prodotti {
			if codice != "" && chiaveCodice(pv.CodiceRichiesto) == codice {
				out = append(out, pv.Rif)
			}
		}
	}
	return out
}

// nomina: la lettura f nomina il prodotto target tg: lo stesso namespace (ConfrontaBasi non lo guarda), una base della
// lettura compatibile con quella del target (uguale, equivalente o compatibile con una forma parziale: A-C07; anche una
// ripetizione della base che non concorda, come in ancoraggio) e il marcatore compatibile con la regola unica (T-B4-06
// rivisto, T-B4-30: due marcatori scritti e diversi sono un'altra identità). Un target senza la lettura del codice non è
// nominato da niente: la base non si inventa (T-B1-07).
func nomina(tg target, f motorea.LetturaForma) bool {
	if tg.lettura == nil || f.Namespace != tg.lettura.Namespace {
		return false
	}
	if a, b := marcatoreDi(*tg.lettura), marcatoreDi(f); a != "" && b != "" && a != b {
		return false
	}
	basi := []motorea.BaseLetta{f.Base}
	for _, r := range f.Ripetizioni {
		if !r.Concorda {
			basi = append(basi, r.Base)
		}
	}
	for _, b := range basi {
		if compatibile(motorea.ConfrontaBasi(tg.lettura.Base, b)) {
			return true
		}
	}
	return false
}

// contestoApplicabile: R106 B, precisata dall'utente il 07/10 (domande-a1c.md). Quando il contesto del messaggio vale
// come ripiego per la pertinenza di un file (R105 [U]: «un file privo di altre evidenze»; E1R §6.4: «senza nessun'altra
// evidenza di pertinenza»; T-E1R-11): per ogni file che conta, non è terminale e non ha evidenze di pertinenza, anche
// con un'identità letta. «Nessuna evidenza collega il file a un prodotto: il contesto del messaggio può fornire una
// pertinenza motivata»; il contesto rende plausibile la pertinenza, e da solo non conferma tipo, identità documentale,
// revisione o capacità di soddisfare un fabbisogno.
//
// L'elenco chiuso delle evidenze che impediscono il ripiego (evidenzeDiPertinenza), ognuna solo se porta a un prodotto
// target, cioè a un elemento del suo perimetro o delle sue strutture:
//  1. un candidato del motore A con l'associazione candidato_unico, ambiguo o discordante, attraverso il target del
//     livello prodotto, le radici raggiungibili o le posizioni (prodottiDelCandidato);
//  2. la collocazione non_determinabile senza candidati, con un codice letto che nomina un target senza struttura;
//  3. una chiave della destinazione F8 (prodottiDellaDestinazione): componente del perimetro di un target, nodo delle
//     strutture di un target o con la riga decisa nel perimetro, codice di un target;
//  4. l'«assegna» su un componente del perimetro di un target;
//  5. un documento confermato su un componente del perimetro di un target;
//  6. uno STEP candidato della fonte di un target, con l'origine motore_a.
//
// Non lo impediscono, perché non collegano il file a un prodotto («una caratteristica generica del file, come
// l'estensione PDF, non è un collegamento a un prodotto»): l'estensione o la natura del file, il tipo documentale
// proposto, la collocazione fuori_richiesta proposta, un codice letto senza candidati (salvo il caso 2), la destinazione
// «generale» (o una chiave che non si risolve), la disponibilità del file. Così un fuori_richiesta proposto, o un codice
// letto che non porta a nessun candidato, allegato a un messaggio che nomina solo P, è pertinente a P e ne tiene aperto lo
// smistamento finché una persona non lo associa, non lo conferma fuori richiesta o non lo scarta (R90: mai verificato in
// silenzio). Un file con evidenze non riceve altre destinazioni dal contesto: la contraddizione si vede con
// valutazione.contesto_discorde (diagnosticaContestoDiscorde). L'identità letta non conta (R106 B): il parametro resta,
// e la prova della funzione lo fissa.
func contestoApplicabile(evidenzePertinenza, identitaLetta bool) bool {
	_ = identitaLetta // con R106 B l'identità letta non conta
	return !evidenzePertinenza
}

// prodottiNominati: R107, precisata dall'utente il 07/10 (domande-a1c.md; la risposta non coincide con una lettera). Quali
// codici «nominano» un prodotto target nel contesto di un file, e di quale messaggio (T-E1R-11; E1R §6.4), con la
// provenienza. L'utente distingue «questo messaggio chiede un prodotto da quotare» da «questo messaggio aiuta a capire a
// quale prodotto si riferiscono una risposta o un allegato»: qui si calcola solo il secondo, come contesto, e una menzione
// non diventa una richiesta (un target diventa confermato solo con l'autorità confermata del DB: target.go).
//
// Il confine di A1c, con la formula dell'opzione A (lettura [T] registrata in domande-a1c.md: la fonte resta
// deterministica, senza agente né LLM; R-71 della revisione di V2): il contesto sono le letture deterministiche del
// motore A sul messaggio a cui il file è allegato, nel segmento corrente e nei segmenti di storia che un caso dichiara
// pertinenti, con il ruolo «prodotto», che nominano un target (nomina). Sono le letture con la funzione «richiesta» del
// router (router-2): l'oggetto e il corpo del corrente (righe 1 e 2), salvo un corrente che un caso dichiara escluso
// (riga 3: menzione), e la storia solo nei segmenti che un caso dichiara pertinenti (riga 4); senza caso la storia è
// menzione (riga 5; R48 A). Il caso aggiunge segmenti, non toglie il corrente. Il gesto 1 non conta, né la controparte o
// la direzione: il perimetro di R93 (b) ha già scelto i file che contano, e l'uso «pertinente» della richiesta
// (RichiestaDelThread) serve alla richiesta, non al contesto. Mai le righe legacy candidato_codice, mai l'agente. Per un
// elemento di un archivio o di un .msg vale il messaggio che porta l'allegato esterno (risalendo i contenitori). Nessuna
// catena di risposte, citazioni o inoltri, nessuna risposta senza codice ricostruita, nessun oggetto uguale, mittente o
// vicinanza temporale: sono del blocco nuovo dell'analisi dei messaggi, fuori da A1c.
//
// Il risultato porta la provenienza (R107: «preservando sempre il messaggio e il segmento»): il messaggio dell'allegato
// esterno, gli ID delle letture che nominano un target (l'ID porta l'unità, quindi il segmento) e i Rif dei prodotti
// nominati, in ordine, senza doppioni. nil se nessun target è nominato; senza grammatica non ci sono letture, quindi nil.
func (s *smistamentoThread) prodottiNominati(a fotorfq.Allegato) *ContestoMessaggio {
	messaggio := s.allegatoEsterno(a).MessaggioID
	mi, ok := s.interpretati[messaggio]
	if !ok {
		return nil
	}
	var prodotti, letture []string
	for _, l := range mi.Interpretazione.Letture {
		if l.Funzione != motorea.FunzRichiesta || !haRuoloProdotto(l.RuoliCandidati) {
			continue
		}
		nominata := false
		for _, tg := range s.targets {
			if nomina(tg, l.Forma) {
				prodotti = append(prodotti, tg.pv.Rif)
				nominata = true
			}
		}
		if nominata {
			letture = append(letture, l.ID)
		}
	}
	if len(prodotti) == 0 {
		return nil
	}
	return &ContestoMessaggio{MessaggioID: messaggio, Letture: ordinatiUnici(letture), Prodotti: ordinatiUnici(prodotti)}
}

// haRuoloProdotto: «prodotto» è fra i ruoli candidati della lettura (una famiglia senza quel ruolo non nomina un
// prodotto: R17 b, come in ProponiProdotti).
func haRuoloProdotto(ruoli []grammatica.Ruolo) bool {
	for _, r := range ruoli {
		if r == grammatica.RuoloProdotto {
			return true
		}
	}
	return false
}

// allegatoEsterno: l'allegato che porta il file nel messaggio, risalendo i contenitori (la voce di un archivio, anche
// annidato, o di un .msg); il file stesso se non ha contenitore. Un contenitore che non c'è, o un giro, fermano la
// risalita all'ultimo allegato trovato.
func (s *smistamentoThread) allegatoEsterno(a fotorfq.Allegato) fotorfq.Allegato {
	visti := map[uuid.UUID]bool{a.ID: true}
	for a.ContenitoreID != nil {
		c := s.col.allegati[*a.ContenitoreID]
		if c == nil || visti[c.ID] {
			break
		}
		visti[c.ID] = true
		a = *c
	}
	return a
}

// pertinenza: la pertinenza del file (contratto §1.5; R93, R105, R106 B e R107 precisate il 07/10). Il contesto del
// messaggio (prodottiNominati), con la sua provenienza, si calcola per ogni file che conta, anche terminale o con
// evidenze (x.messaggio, poi AssociazioneFile.Contesto): «una contraddizione deve restare visibile», e i riferimenti
// possibili di un messaggio che nomina più prodotti si conservano anche quando il file non è orfano. La regola della
// pertinenza non cambia: le evidenze, poi, solo per un file che conta, non è terminale e non ha evidenze
// (contestoApplicabile), il contesto come ripiego: un solo prodotto nominato dà la pertinenza con l'evidenza
// contesto_messaggio (PertinenzaContesto); più prodotti nominati non legano il file a nessuno, e il file è orfano con i
// prodotti nominati nell'avviso (ProdottiContesto: «senza rendere automaticamente il file pertinente a tutti e senza
// bloccare tutti indistintamente»); nessun prodotto, nessuna evidenza. Per un file terminale il contesto non dà
// pertinenza: la decisione registrata non si sostituisce. Orfano: conta, non è terminale, non è pertinente a nessun
// prodotto, e la pertinenza si calcola (pertinenzaCalcolata: R-75 della revisione di V2). Con una sezione dello
// smistamento assente (T-12: lo scarto, la destinazione o un documento non si vedono) o senza grammatica (T-B6-61:
// niente candidati né contesto) un file senza evidenze non si dichiara orfano: la pertinenza è incompleta, lo
// smistamento dei prodotti non è calcolato, e il file sta in «da smistare» con il suo motivo, senza l'avviso. I prodotti
// nominati (ProdottiContesto) restano solo per l'orfano. I prodotti con cui il contesto si confronta (x.collegati) sono
// quelli delle evidenze o, per un file terminale, della decisione (prodottiCollegati).
func (s *smistamentoThread) pertinenza(x *fileDelThread) {
	evidenze := s.evidenzeDiPertinenza(x)
	if x.conta() {
		x.messaggio = s.prodottiNominati(x.a)
	}
	if x.messaggio != nil && !x.terminale && contestoApplicabile(len(evidenze) > 0, x.af != nil && len(x.af.Letture) > 0) {
		switch n := x.messaggio.Prodotti; len(n) {
		case 1:
			x.contesto = append([]string(nil), n...)
		default:
			x.nominati = append([]string(nil), n...)
		}
	}
	x.pertinente = ordinatiUnici(append(evidenze, x.contesto...))
	x.orfano = x.conta() && !x.terminale && len(x.pertinente) == 0 && s.pertinenzaCalcolata()
	if !x.orfano {
		x.nominati = nil
	}
	x.collegati = s.prodottiCollegati(x, evidenze)
}

// prodottiCollegati: i prodotti target a cui il file è già collegato, con cui si confronta il contesto del messaggio
// (R106 B, precisata il 07/10: «Il file ha già evidenze o decisioni che lo collegano a un prodotto»):
//   - per un file che non è terminale, i prodotti delle evidenze di pertinenza (evidenzeDiPertinenza: anche l'«assegna»
//     e la destinazione F8, che sono decisioni non confermate);
//   - per un file terminale, i prodotti della decisione: quelli con il componente del documento confermato che lo porta
//     nel perimetro (prodottiDelComponente). Un documento generale (senza componente), un duplicato o uno scarto non
//     collegano il file a un prodotto (dubbio T-B6-172).
//
// In ordine, senza doppioni; nil se il file non conta.
func (s *smistamentoThread) prodottiCollegati(x *fileDelThread, daEvidenze []string) []string {
	if !x.conta() {
		return nil
	}
	if !x.terminale {
		return ordinatiUnici(append([]string(nil), daEvidenze...))
	}
	if x.documento != nil && x.documento.ComponenteID != nil {
		return ordinatiUnici(s.prodottiDelComponente(*x.documento.ComponenteID))
	}
	return nil
}

// diagnosticaContestoDiscorde: valutazione.contesto_discorde per un file il cui messaggio nomina prodotti target che non
// hanno niente in comune con quelli a cui il file è già collegato (prodottiCollegati) (R106 B, precisata il 07/10: «Il
// file ha già evidenze o decisioni che lo collegano a un prodotto: il contesto non deve aggiungere indiscriminatamente
// altre destinazioni. Una contraddizione deve restare visibile»). Un avviso, di natura dati: non blocca niente, non cambia
// la pertinenza né la decisione, e non entra nell'impronta del prodotto (che copre solo dati decisi: impronta.go). Solo
// quando la pertinenza si calcola (pertinenzaCalcolata: con T-12 le evidenze possono mancare, e una contraddizione non si
// dichiara senza saperlo; dubbio T-B6-171). Un messaggio che nomina più prodotti, uno dei quali è collegato, non è una
// contraddizione. I riferimenti (dubbio T-B6-173): l'allegato, poi «contesto:<rif>» per ogni prodotto nominato, poi
// «evidenza:<rif>» (o «decisione:<rif>» per un file terminale) per ogni prodotto collegato. ok falso se non c'è
// contraddizione.
func (s *smistamentoThread) diagnosticaContestoDiscorde(x *fileDelThread) (evidenze.Diagnostica, bool) {
	if !s.pertinenzaCalcolata() || x.messaggio == nil || len(x.collegati) == 0 {
		return evidenze.Diagnostica{}, false
	}
	for _, p := range x.messaggio.Prodotti {
		if contiene(x.collegati, p) {
			return evidenze.Diagnostica{}, false
		}
	}
	rif := []string{x.a.ID.String()}
	for _, p := range x.messaggio.Prodotti {
		rif = append(rif, prefissoRifContesto+p)
	}
	prefisso := prefissoRifEvidenza
	if x.terminale {
		prefisso = prefissoRifDecisione
	}
	for _, p := range x.collegati {
		rif = append(rif, prefisso+p)
	}
	return evidenze.Diagnostica{
		Codice:   CodiceContestoDiscorde,
		Gravita:  evidenze.GravitaAvviso,
		Natura:   evidenze.NaturaDati,
		Percorso: "associazioni[" + x.a.ID.String() + "].contesto",
		Messaggio: "il messaggio del file nomina prodotti che non hanno niente in comune con quelli a cui il file è già collegato " +
			"(per le evidenze o, per un file terminale, per la decisione): il contesto non aggiunge destinazioni e non sostituisce " +
			"la decisione, e la contraddizione resta visibile (R106 B); non blocca niente",
		Rif: rif,
	}, true
}

// I prefissi dei riferimenti di valutazione.contesto_discorde (dubbio T-B6-173): il ruolo di ogni Rif di prodotto.
const (
	prefissoRifContesto  = "contesto:"
	prefissoRifEvidenza  = "evidenza:"
	prefissoRifDecisione = "decisione:"
)

// pertinenzaCalcolata: la pertinenza dei file del thread si calcola: le sezioni dello smistamento ci sono (T-12) e c'è
// la grammatica (T-B6-61). Senza, nessun file è orfano (R-75) e lo smistamento dei prodotti non è calcolato
// (ingressoDelProdotto).
func (s *smistamentoThread) pertinenzaCalcolata() bool { return !s.t12 && s.m != nil }

// voceDaSmistare: R108 A, confermata dall'utente il 07/10 con il requisito operativo per lo spazio di verifica
// (domande-a1c.md). Chi entra in «da smistare», e con quale motivo (contratto §1.5, «da smistare»; §2.3 e E1: il motivo è
// uno dei cinque): «da smistare» ha solo i file che contano e non sono terminali con uno dei cinque motivi, più gli
// orfani. Il requisito operativo (vedere, correggere e confermare insieme nella Distinta; distinguere assenza di
// destinazione, proposta da confermare, analisi pendente o fallita) è dello spazio di verifica e del contratto dei DTO di
// B7, non di questa funzione: un candidato unico e plausibile resta da confermare, e si vede nello smistamento del suo
// prodotto. Il motivo viene dai due assi della proposta, il primo che vale:
//   - per un orfano senza candidati, la tabella di E1 (§2.5, FileDaSmistare; R-74 della revisione di V2):
//     fuori_richiesta_non_confermato se la collocazione proposta è fuori richiesta, altrimenti nessun_candidato, anche
//     per un discordante (la discordanza resta visibile in AssociazioneFile.Associazione);
//   - associazione_discordante, associazione_ambigua (l'associazione);
//   - collocazione_non_determinabile (la collocazione), quando il file ha candidati (livelli diversi) o un codice letto
//     (il target senza struttura); un file senza nessuna lettura d'identità è nessun_candidato, perché da lui non si
//     legge niente da collocare (dubbio T-B6-63);
//   - fuori_richiesta_non_confermato (la collocazione proposta fuori richiesta, mai uno scarto);
//   - nessun_candidato: nessun candidato, anche per un file che l'adattatore non legge (non_valutata).
//
// Un candidato_unico non confermato (la pre-associazione), un'associazione confermata in conflitto, un nuovo file su un
// componente deciso o un conflitto identita_documento non hanno uno dei cinque motivi: il file non entra, e si vede nello
// smistamento del suo prodotto (FileNonTerminali, Motivi) e nelle associazioni (Conflitto). Un orfano entra sempre, con
// uno dei cinque.
func voceDaSmistare(x *fileDelThread) (MotivoSmistamento, bool) {
	if !x.conta() || x.terminale {
		return "", false
	}
	associazione, collocazione := ancoraggio.AssociazioneNonValutata, ancoraggio.Collocazione("")
	candidati, letture := false, false
	if x.af != nil {
		associazione, collocazione = x.af.Associazione, x.af.Collocazione
		candidati, letture = len(x.af.Candidati) > 0, len(x.af.Letture) > 0
	}
	var motivo MotivoSmistamento
	switch {
	case x.orfano && !candidati && collocazione == ancoraggio.CollocazioneFuoriRichiesta:
		motivo = MotivoSmistamentoFuoriRichiestaNonConfermato
	case x.orfano && !candidati:
		motivo = MotivoSmistamentoNessunCandidato
	case associazione == ancoraggio.AssociazioneDiscordante:
		motivo = MotivoSmistamentoAssociazioneDiscordante
	case associazione == ancoraggio.AssociazioneAmbiguo:
		motivo = MotivoSmistamentoAssociazioneAmbigua
	case collocazione == ancoraggio.CollocazioneNonDeterminabile && (candidati || letture):
		motivo = MotivoSmistamentoCollocazioneNonDeterminabile
	case collocazione == ancoraggio.CollocazioneFuoriRichiesta:
		motivo = MotivoSmistamentoFuoriRichiestaNonConfermato
	case associazione != ancoraggio.AssociazioneCandidatoUnico:
		motivo = MotivoSmistamentoNessunCandidato
	}
	if x.orfano && motivo == "" {
		motivo = MotivoSmistamentoNessunCandidato
	}
	return motivo, x.orfano || motivo != ""
}

// avvisiDegliOrfani: gli orfani del thread per il fascicolo (R93: un avviso, che non cambia Congelabile; T-E1-16;
// F0-18): il numero e, per ogni orfano in ordine di allegato, «orfano:<allegato_id>», oppure, con i prodotti nominati
// dal messaggio, «orfano:<allegato_id>:prodotti_contesto:<rif>|<rif>» (il separatore dei Rif è «|», perché i Rif hanno
// già i due punti). Uno scartato non è un orfano (è terminale), quindi non fa un avviso.
func avvisiDegliOrfani(daSmistare []FileDaSmistare) (int, []string) {
	n := 0
	var out []string
	for _, f := range daSmistare {
		if !f.Orfano {
			continue
		}
		n++
		a := prefissoAvvisoOrfano + f.AllegatoID.String()
		if len(f.ProdottiContesto) > 0 {
			a += separatoreProdottiContesto + strings.Join(f.ProdottiContesto, "|")
		}
		out = append(out, a)
	}
	return n, out
}

// Il formato degli avvisi degli orfani (F0-18).
const (
	prefissoAvvisoOrfano       = "orfano:"
	separatoreProdottiContesto = ":prodotti_contesto:"
)
