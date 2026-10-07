package valutazione

import (
	"sort"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
)

// smistamento.go: l'asse 5, lo smistamento dei file ai prodotti (R81, R85, R91; contratto §1.5, §2.3, §2.5, §2.6; fase 0
// di B6, F.2, F.4). I tipi li ha dichiarati la fase V1, perché l'esito del thread e il prodotto li portano (EsitoThread,
// ProdottoValutato), e la fase 0 li ha congelati per le corsie B e C. La fase V2 aggiunge qui la regola pura dello
// smistamento di un prodotto (Smistamento), su un ingresso astratto (IngressoSmistamento: T-B0-22), con i percorsi di
// revisione tecnica come ingresso (PercorsoRevisione: in A1c nessun adattatore li produce, LD-18, e Calcola passa nil).
// L'adattatore che dalla fotografia fa le associazioni dei file, il perimetro, la pertinenza, lo scarto, gli orfani,
// «da smistare» e i conflitti dell'asse è in associazioni.go e pertinenza.go.

// StatoSmistamento: lo stato dell'asse 5 (contratto §2.3; E1: in_revisione).
type StatoSmistamento string

const (
	SmistamentoDaVerificare StatoSmistamento = "da_verificare"
	SmistamentoVerificato   StatoSmistamento = "verificato"
	SmistamentoConflitto    StatoSmistamento = "conflitto"
	SmistamentoInRevisione  StatoSmistamento = "in_revisione"
)

// MotivoSmistamento: perché lo smistamento non è verificato (contratto §2.3, nove valori; §2.5, +1; §2.6, +1).
type MotivoSmistamento string

const (
	MotivoSmistamentoFileDaSmistare               MotivoSmistamento = "file_da_smistare"
	MotivoSmistamentoNessunCandidato              MotivoSmistamento = "nessun_candidato"
	MotivoSmistamentoAssociazioneAmbigua          MotivoSmistamento = "associazione_ambigua"
	MotivoSmistamentoAssociazioneDiscordante      MotivoSmistamento = "associazione_discordante"
	MotivoSmistamentoCollocazioneNonDeterminabile MotivoSmistamento = "collocazione_non_determinabile"
	MotivoSmistamentoFuoriRichiestaNonConfermato  MotivoSmistamento = "fuori_richiesta_non_confermato"
	MotivoSmistamentoAssociazioneNonConfermata    MotivoSmistamento = "associazione_non_confermata"
	MotivoSmistamentoAssociazioneInConflitto      MotivoSmistamento = "associazione_in_conflitto"
	MotivoSmistamentoNuovoFile                    MotivoSmistamento = "nuovo_file_su_componente_deciso"
	MotivoSmistamentoRevisioneTecnicaAperta       MotivoSmistamento = "revisione_tecnica_aperta"
	MotivoSmistamentoIdentitaDocumento            MotivoSmistamento = "identita_documento_in_conflitto"
)

// ordineMotiviSmistamento: l'ordine canonico dei motivi in VerificaSmistamento.Motivi, quello della dichiarazione.
var ordineMotiviSmistamento = []MotivoSmistamento{
	MotivoSmistamentoFileDaSmistare, MotivoSmistamentoNessunCandidato, MotivoSmistamentoAssociazioneAmbigua,
	MotivoSmistamentoAssociazioneDiscordante, MotivoSmistamentoCollocazioneNonDeterminabile, MotivoSmistamentoFuoriRichiestaNonConfermato,
	MotivoSmistamentoAssociazioneNonConfermata, MotivoSmistamentoAssociazioneInConflitto, MotivoSmistamentoNuovoFile,
	MotivoSmistamentoRevisioneTecnicaAperta, MotivoSmistamentoIdentitaDocumento,
}

// VerificaSmistamento: l'asse 5 di un prodotto (contratto §2.3, §2.5).
//   - FileNonTerminali: i file pertinenti al prodotto che non sono in uno stato terminale (T-B0-32), in ordine di ID.
//   - Motivi: un motivo per ogni condizione che manca, nell'ordine dei valori (ordineMotiviSmistamento).
//   - Conflitti: i Conflitto.Rif dell'asse smistamento, come VerificaAsse.Conflitti, in ordine, senza doppioni.
//   - Calcolata: falsa con T-12 (una sezione assente negli export), per il target senza componente (T-B6-07, F0-17) e
//     per un thread senza grammatica (T-B6-61, confermato dalla revisione di V2: senza grammatica non ci sono candidati
//     né contesto, e un prodotto con tutti i file noti terminali risulterebbe verificato in silenzio, R90); allora lo
//     stato è da_verificare, mai verificato. Con T-12 sulle sezioni dello smistamento, o senza grammatica, nessun file
//     è orfano (R-75 della revisione di V2).
//   - Percorsi: gli ID dei percorsi di revisione tecnica aperti che coinvolgono il prodotto (E1).
type VerificaSmistamento struct {
	Stato            StatoSmistamento    `json:"stato"`
	FileNonTerminali []uuid.UUID         `json:"file_non_terminali,omitempty"`
	Motivi           []MotivoSmistamento `json:"motivi,omitempty"`
	Conflitti        []string            `json:"conflitti,omitempty"`
	Calcolata        bool                `json:"calcolata"`
	Percorsi         []string            `json:"percorsi,omitempty"`
}

// I valori di AssociazioneFile.Perimetro (contratto §2.5; T-E1-12).
const (
	PerimetroDentro              = "dentro"
	PerimetroInline              = "inline"
	PerimetroElementoOutlook     = "elemento_outlook"
	PerimetroContenitoreEstratto = "contenitore_estratto"
	PerimetroCollegamento        = "collegamento"
	PerimetroMessaggioInUscita   = "messaggio_in_uscita"
	PerimetroAltraControparte    = "altra_controparte"
)

// AssociazioneFile: l'associazione di un allegato del thread (6.0.7, R81; contratto §2.3, §2.5, §2.6), uno per allegato,
// in ordine di AllegatoID. La calcola l'adattatore di associazioni.go.
//   - Associazione: non_valutata per i contenitori e per le nature che non sono «file».
//   - Proposta: «<Livello>/<Target>» per candidato (F0-12): la chiave di un candidato è (Livello, Target), mai il solo
//     Target (T-B4-28).
//   - DestinazioneF8: le chiavi di dettagli.destinazione; nil negli export. Manuale: l'«assegna» della proposta.
//     Confermata, DocumentoID: il documento confermato. Origine: la più forte, confermato > manuale > proposto.
//   - DestinazioneCoincide: nil = non determinabile. Esclusa: solo una proposta scartata con deciso_da (T-E1R-10).
//     Terminale: T-B0-32. Pertinente, PertinenzaContesto: i Rif dei prodotti (R106 B e R107, precisate dall'utente il
//     07/10, domande-a1c.md).
//   - Contesto: il contesto del messaggio del file, con la sua provenienza (ContestoMessaggio), per ogni file che conta
//     il cui messaggio nomina almeno un prodotto target, anche terminale o con evidenze (R106 B e R107, precisate il
//     07/10: il contesto resta distinto dall'associazione, e una contraddizione resta visibile). Non cambia la
//     pertinenza: PertinenzaContesto resta il solo effetto del contesto sulla regola, e solo per un file senza evidenze.
type AssociazioneFile struct {
	AllegatoID           uuid.UUID               `json:"allegato_id"`
	Associazione         ancoraggio.Associazione `json:"associazione"`
	Collocazione         ancoraggio.Collocazione `json:"collocazione,omitempty"`
	Proposta             []string                `json:"proposta,omitempty"`
	DestinazioneF8       []string                `json:"destinazione_f8,omitempty"`
	Manuale              *uuid.UUID              `json:"manuale,omitempty"`
	Confermata           *uuid.UUID              `json:"confermata,omitempty"`
	DocumentoID          *uuid.UUID              `json:"documento_id,omitempty"`
	Origine              ancoraggio.OrigineDato  `json:"origine,omitempty"`
	DestinazioneCoincide *bool                   `json:"destinazione_coincide,omitempty"`
	Conflitto            bool                    `json:"conflitto"`
	Esclusa              bool                    `json:"esclusa"`
	Terminale            bool                    `json:"terminale"`
	Pertinente           []string                `json:"pertinente,omitempty"`
	Perimetro            string                  `json:"perimetro"`
	PertinenzaContesto   []string                `json:"pertinenza_contesto,omitempty"`
	Contesto             *ContestoMessaggio      `json:"contesto,omitempty"`
}

// ContestoMessaggio: il contesto del messaggio di un file e da dove viene (R106 B e R107, precisate dall'utente il 07/10,
// domande-a1c.md: «preservando sempre il messaggio e il segmento da cui deriva l'informazione»). Lo calcola
// prodottiNominati (pertinenza.go).
//   - MessaggioID: il messaggio dell'allegato esterno (per la voce di un archivio o di un .msg, il messaggio che porta il
//     contenitore).
//   - Letture: gli ID delle letture del motore A su quel messaggio che nominano un prodotto target, in ordine, senza
//     doppioni. L'ID di una lettura porta l'unità, quindi il segmento («l:<unità>:…»); vale insieme a MessaggioID.
//   - Prodotti: i Rif dei prodotti target nominati, in ordine, senza doppioni; mai vuoto (senza prodotti nominati il
//     contesto non c'è).
//
// È informazione: non è un candidato, non soddisfa voci, non conferma niente, e non entra nell'impronta del prodotto.
type ContestoMessaggio struct {
	MessaggioID uuid.UUID `json:"messaggio_id"`
	Letture     []string  `json:"letture"`
	Prodotti    []string  `json:"prodotti"`
}

// FileDaSmistare: un file da smistare del thread (R85, R91; contratto §2.3, §2.5, §2.6). Lo calcola l'adattatore di
// associazioni.go, con R108 A, confermata dall'utente con il requisito operativo per lo spazio di verifica (pertinenza.go,
// voceDaSmistare).
//   - Motivo: uno dei cinque (nessun_candidato, associazione_ambigua, associazione_discordante,
//     collocazione_non_determinabile, fuori_richiesta_non_confermato; R108 A).
//   - Prodotti: la pertinenza per evidenza; vuoto = orfano (T-E1-11), salvo quando la pertinenza non si calcola (T-12
//     sulle sezioni dello smistamento, thread senza grammatica: R-75 della revisione di V2), e allora Orfano è falso e
//     il file non fa l'avviso. Proposta: «<Livello>/<Target>» del solo candidato con candidato_unico, altrimenti ""
//     (F0-12; con R108 A un candidato_unico non entra, quindi Proposta è sempre ""). ProdottiContesto: i prodotti
//     nominati dal messaggio, quando sono più d'uno, solo per un orfano (F0-18).
type FileDaSmistare struct {
	AllegatoID       uuid.UUID         `json:"allegato_id"`
	NomeFile         string            `json:"nome_file"`
	Motivo           MotivoSmistamento `json:"motivo"`
	Prodotti         []string          `json:"prodotti,omitempty"`
	Proposta         string            `json:"proposta,omitempty"`
	Orfano           bool              `json:"orfano"`
	ProdottiContesto []string          `json:"prodotti_contesto,omitempty"`
}

// ---- i percorsi di revisione tecnica (E1; T-E1-13) ----

// OriginePercorso: da dove nasce un percorso di revisione tecnica (contratto §2.5): la fattibilità, il flusso dell'Inbox
// per un nuovo CAD, l'operatore.
type OriginePercorso string

const (
	OriginePercorsoFattibilita   OriginePercorso = "fattibilita"
	OriginePercorsoInboxNuovoCAD OriginePercorso = "inbox_nuovo_cad"
	OriginePercorsoOperatore     OriginePercorso = "operatore"
)

// PercorsoRevisione: un percorso di revisione tecnica (contratto §2.5; T-E1-13): l'ingresso astratto della regola dello
// smistamento, come GestiVerificaBOM. In A1c nessun adattatore lo produce, perché il DB non ha percorsi (LD-18): Calcola
// passa nil, e la regola si prova con percorsi sintetici (PO-24, PO-25). Lo aprirà e lo chiuderà un gesto del servizio
// futuro, mai un conflitto o un file (contratto §0 punto 9).
//   - Elementi: i Rif di prodotto, componente o nodo coinvolti.
//   - ApertoDa, ApertoIl: chi e quando l'ha aperto (F0-15: un percorso aperto ha sempre chi e quando); ChiusoDa e
//     ChiusoIl solo quando è chiuso.
type PercorsoRevisione struct {
	ID                 string          `json:"id"`
	Origine            OriginePercorso `json:"origine"`
	Motivo             string          `json:"motivo"`
	Elementi           []string        `json:"elementi"`
	CondizioneChiusura string          `json:"condizione_chiusura"`
	Aperto             bool            `json:"aperto"`
	ApertoDa           uuid.UUID       `json:"aperto_da"`
	ChiusoDa           *uuid.UUID      `json:"chiuso_da,omitempty"`
	ApertoIl           time.Time       `json:"aperto_il"`
	ChiusoIl           *time.Time      `json:"chiuso_il,omitempty"`
}

// ---- la regola dello smistamento di un prodotto (R81; contratto §1.5; T-B0-22) ----

// IngressoSmistamento: l'ingresso astratto della regola dello smistamento di un prodotto P (T-B0-22; F.4). In A1c lo
// costruisce l'adattatore dalla fotografia (associazioni.go); la regola si prova anche con ingressi sintetici.
//   - Prodotto: il Rif di P. Elementi: i Rif degli elementi del suo perimetro che un percorso può nominare (i componenti
//     della BOM confermata, i nodi delle strutture che contano); il Rif di P conta sempre.
//   - File: i file pertinenti a P (la pertinenza per evidenza, con il contesto del messaggio: pertinenza.go), con lo
//     stato terminale (T-B0-32), i due assi della proposta e l'«assegna».
//   - Voci: le voci certe della completezza di P (CompletezzaDocumentale.Voci, cioè VociCerte(P)), per il primo
//     requisito (T-B6-08).
//   - Conflitti: i conflitti aperti sullo smistamento di P (asse smistamento, con il prodotto P; R95 A).
//   - Calcolata: falsa quando l'asse non si può verificare (T-12, il target senza componente: T-B6-07 e F0-17; il thread
//     senza grammatica: T-B6-61).
type IngressoSmistamento struct {
	Prodotto  string                 `json:"prodotto"`
	Elementi  []string               `json:"elementi,omitempty"`
	File      []FileDelloSmistamento `json:"file,omitempty"`
	Voci      []VoceFabbisogno       `json:"voci,omitempty"`
	Conflitti []Conflitto            `json:"conflitti,omitempty"`
	Calcolata bool                   `json:"calcolata"`
}

// FileDelloSmistamento: un file pertinente al prodotto, come lo vede la regola: terminale o no (T-B0-32), l'associazione
// e la collocazione proposte dal motore A (non_valutata e vuota per un file che l'adattatore non legge), l'«assegna»
// non confermato.
type FileDelloSmistamento struct {
	AllegatoID   uuid.UUID               `json:"allegato_id"`
	Terminale    bool                    `json:"terminale"`
	Associazione ancoraggio.Associazione `json:"associazione"`
	Collocazione ancoraggio.Collocazione `json:"collocazione,omitempty"`
	Manuale      bool                    `json:"manuale"`
}

// Smistamento: la regola dello smistamento di un prodotto P (R81, T-B0-25; E1: T-E1-11, T-E1-13; R95 A; contratto §1.5,
// il blocco SmistamentoVerificato), su un ingresso astratto e con i percorsi di revisione tecnica come ingresso.
//
//	SmistamentoVerificato(P) =
//	      ogni documento presente richiesto da VociCerte(P) è associato con decisione confermata (T-B6-08: una voce
//	      «da_verificare» con il motivo associazione_non_confermata; un documento che manca del tutto lo dice la
//	      completezza, non lo smistamento)
//	  AND nessun file pertinente a P, senza decisione, ha Associazione ∈ {ambiguo, discordante} o Collocazione =
//	      non_determinabile (R64 A: il discordante blocca)
//	  AND ogni file pertinente a P è terminale (T-B0-32)
//	  AND nessun conflitto aperto sullo smistamento di P (R95 A)
//	  AND nessun percorso di revisione aperto che coinvolge P (T-E1-13)
//
// I motivi, uno per ogni condizione che manca, nell'ordine dei valori:
//   - associazione_non_confermata per una voce certa con un documento presente non confermato;
//   - per ogni file pertinente non terminale i motivi dei suoi assi (motiviDelFile) e, una volta, file_da_smistare;
//   - per ogni conflitto il motivo del suo tipo (associazione, nuovo_file, identita_documento);
//   - revisione_tecnica_aperta per un percorso aperto che nomina P o un suo elemento.
//
// Lo stato: da_verificare se l'asse non si può verificare (in.Calcolata falso: T-12, T-B6-07, F0-17, T-B6-61), con le
// condizioni calcolate e mostrate lo stesso; altrimenti in_revisione con un percorso aperto (vince sugli altri: T-E1-13),
// poi conflitto con un conflitto aperto (R95 A), poi verificato se non manca niente, altrimenti da_verificare. Le
// condizioni si calcolano e si mostrano anche con lo stato in_revisione: servono a chiudere il percorso. Mai verificato
// per difetto. È pura e deterministica: l'ordine degli ingressi non conta, e gli ingressi non cambiano.
func Smistamento(in IngressoSmistamento, percorsi []PercorsoRevisione) VerificaSmistamento {
	out := VerificaSmistamento{Calcolata: in.Calcolata}
	motivi := map[MotivoSmistamento]bool{}
	for _, v := range in.Voci {
		if v.Esito == EsitoDaVerificare && v.Motivo == MotivoFabbisognoAssociazioneNonConfermata {
			motivi[MotivoSmistamentoAssociazioneNonConfermata] = true
		}
	}

	file := append([]FileDelloSmistamento(nil), in.File...)
	sort.SliceStable(file, func(i, j int) bool { return file[i].AllegatoID.String() < file[j].AllegatoID.String() })
	visti := map[uuid.UUID]bool{}
	for _, f := range file {
		if f.Terminale || visti[f.AllegatoID] {
			continue
		}
		visti[f.AllegatoID] = true
		out.FileNonTerminali = append(out.FileNonTerminali, f.AllegatoID)
		for _, m := range motiviDelFile(f) {
			motivi[m] = true
		}
	}
	if len(out.FileNonTerminali) > 0 {
		motivi[MotivoSmistamentoFileDaSmistare] = true
	}

	var rif []string
	for _, c := range in.Conflitti {
		if c.Asse != AsseSmistamento || c.Prodotto != in.Prodotto {
			continue
		}
		rif = append(rif, c.Rif)
		if m := motivoDelConflitto(c.Tipo); m != "" {
			motivi[m] = true
		}
	}
	out.Conflitti = ordinatiUnici(rif)

	elementi := map[string]bool{in.Prodotto: true}
	for _, e := range in.Elementi {
		elementi[e] = true
	}
	var aperti []string
	for _, p := range percorsi {
		if !p.Aperto {
			continue
		}
		for _, e := range p.Elementi {
			if elementi[e] {
				aperti = append(aperti, p.ID)
				break
			}
		}
	}
	out.Percorsi = ordinatiUnici(aperti)
	if len(out.Percorsi) > 0 {
		motivi[MotivoSmistamentoRevisioneTecnicaAperta] = true
	}

	for _, m := range ordineMotiviSmistamento {
		if motivi[m] {
			out.Motivi = append(out.Motivi, m)
		}
	}
	switch {
	case !in.Calcolata:
		out.Stato = SmistamentoDaVerificare
	case len(out.Percorsi) > 0:
		out.Stato = SmistamentoInRevisione
	case len(out.Conflitti) > 0:
		out.Stato = SmistamentoConflitto
	case len(out.Motivi) == 0:
		out.Stato = SmistamentoVerificato
	default:
		out.Stato = SmistamentoDaVerificare
	}
	return out
}

// motiviDelFile: i motivi di un file pertinente non terminale, uno per asse (T-B0-25: i nomi qualificati, LD-06).
//   - L'associazione: discordante e ambiguo bloccano (seconda condizione; R64 A); candidato_unico è una proposta, la
//     pre-associazione (R85), che aspetta la conferma: associazione_non_confermata; senza candidati (anche un file che
//     l'adattatore non legge, non_valutata): nessun_candidato.
//   - La collocazione: non_determinabile blocca (seconda condizione); fuori_richiesta è una proposta, mai uno scarto
//     (T-B0-25, R105): fuori_richiesta_non_confermato.
//   - L'«assegna» non confermato: associazione_non_confermata (T-B0-12: un'associazione manuale non è una conferma).
func motiviDelFile(f FileDelloSmistamento) []MotivoSmistamento {
	var out []MotivoSmistamento
	switch f.Associazione {
	case ancoraggio.AssociazioneDiscordante:
		out = append(out, MotivoSmistamentoAssociazioneDiscordante)
	case ancoraggio.AssociazioneAmbiguo:
		out = append(out, MotivoSmistamentoAssociazioneAmbigua)
	case ancoraggio.AssociazioneCandidatoUnico:
		out = append(out, MotivoSmistamentoAssociazioneNonConfermata)
	default:
		out = append(out, MotivoSmistamentoNessunCandidato)
	}
	switch f.Collocazione {
	case ancoraggio.CollocazioneNonDeterminabile:
		out = append(out, MotivoSmistamentoCollocazioneNonDeterminabile)
	case ancoraggio.CollocazioneFuoriRichiesta:
		out = append(out, MotivoSmistamentoFuoriRichiestaNonConfermato)
	}
	if f.Manuale {
		out = append(out, MotivoSmistamentoAssociazioneNonConfermata)
	}
	return out
}

// motivoDelConflitto: il motivo dello smistamento per un conflitto del suo asse; "" per un tipo che non sta su questo asse.
func motivoDelConflitto(t TipoConflitto) MotivoSmistamento {
	switch t {
	case ConflittoAssociazione:
		return MotivoSmistamentoAssociazioneInConflitto
	case ConflittoNuovoFile:
		return MotivoSmistamentoNuovoFile
	case ConflittoIdentitaDocumento:
		return MotivoSmistamentoIdentitaDocumento
	}
	return ""
}
