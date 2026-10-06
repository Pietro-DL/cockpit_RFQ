package valutazione

import (
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
)

// La verifica della BOM, gli assi 3 e 4 (B5, fase 1; R80; contratto §1.0 righe 3 e 4, §1.3, §2.3, §2.6; T-B0-07,
// T-B0-22, T-B0-24; R61 A, R95 A; K-02 con la lettura A, confermata dall'utente [U], T-E1R-04). BOMVerificata =
// NomenclaturaVerificata AND GerarchiaVerificata. La regola (VerificaDellaBOM) è una funzione pura su due ingressi astratti, i gesti di verifica
// (GestiVerificaBOM) e la struttura del prodotto (StrutturaDaVerificare); il legacy entra solo dal suo adattatore
// (gestiDaConfermaLAlbero), e la struttura la prepara strutturaDaVerificare dalla fotografia e dalle strutture di
// ancoraggio (T-B0-22). Nessuna verifica si inventa: verificata viene solo da un gesto salvato (contratto §0 n.6).

// GestiVerificaBOM: i gesti di verifica della BOM di un prodotto, l'ingresso astratto della regola (contratto §2.3;
// T-B0-22). In A1c l'unico produttore è l'adattatore di «Conferma l'albero» (Legacy vero), che dà lo stesso gesto per
// nomenclatura e gerarchia, perché un solo gesto le ha confermate tutte e due (R80); nello spazio di verifica sarà il
// gesto nuovo (R100 A, Legacy falso), con due controlli distinti anche nella stessa schermata (R80, E1). nil vuol dire
// «nessun gesto».
type GestiVerificaBOM struct {
	Nomenclatura *GestoVerifica `json:"nomenclatura,omitempty"`
	Gerarchia    *GestoVerifica `json:"gerarchia,omitempty"`
	Legacy       bool           `json:"legacy"`
}

// GestoVerifica: un gesto di verifica, con chi e quando e che cosa copre (contratto §2.3).
//   - Da, Il: chi e quando, come il DB li dice (per il legacy, il segno di «Conferma l'albero»: il suo «da» e il suo
//     «il», testo RFC 3339 com'è nel JSON).
//   - Nodi, Archi: che cosa il gesto copre. Nel modello nuovo i Rif dei nodi della struttura del prodotto (RifNodo) e
//     degli archi (RifArco fra due RifNodo); per il legacy i componenti e le relazioni confermate che il segno dice
//     (RifComponente, RifArco fra due RifComponente), perché il segno non ha né STEP né sha256 né radice (R61 A).
type GestoVerifica struct {
	Da    *uuid.UUID `json:"da,omitempty"`
	Il    string     `json:"il,omitempty"`
	Nodi  []string   `json:"nodi,omitempty"`
	Archi []string   `json:"archi,omitempty"`
}

// StatoVerificaAsse: lo stato degli assi 3 e 4 (R79, R80; contratto §1.0, §2.3; era StatoVerificaBOM).
//   - da_verificare: manca il gesto, o resta qualcosa da decidere, o (K-02, lettura A) la gerarchia è senza la fonte
//     confermata;
//   - verificata: un gesto salvato copre tutto, niente resta da decidere, nessun conflitto;
//   - conflitto: una decisione che una proposta nuova contraddice (T-B0-24): blocca l'asse finché non è risolta, e la
//     decisione resta (R95 A);
//   - non_verificabile: solo nelle letture legacy, quando il DB non ha un gesto da leggere (un prodotto senza figli,
//     R80; un target senza riga componente, T-B0-07).
type StatoVerificaAsse string

const (
	StatoAsseDaVerificare    StatoVerificaAsse = "da_verificare"
	StatoAsseVerificata      StatoVerificaAsse = "verificata"
	StatoAsseConflitto       StatoVerificaAsse = "conflitto"
	StatoAsseNonVerificabile StatoVerificaAsse = "non_verificabile"
)

// I motivi di un asse non verificato (VerificaAsse.Motivo; il contratto fissa solo fonte_non_confermata, gli altri sono
// della fase 1 di B5, dubbio T-B5-04):
//   - conflitto: l'asse ha almeno un conflitto (VerificaAsse.Conflitti);
//   - target_senza_componente: il target non ha la riga componente, quindi nessuna BOM da leggere (T-B0-07);
//   - senza_figli_nessun_gesto: legacy, «nessun gesto di verifica per un prodotto senza figli» (R80, R71 A): senza
//     figli né confermati né proposti il DB non ha un gesto da leggere;
//   - nessun_gesto: nessun gesto di verifica per l'asse;
//   - nessuna_struttura: modello nuovo, il prodotto non ha una struttura da coprire;
//   - da_decidere: il gesto c'è, ma restano righe da decidere o elementi che il gesto non copre (DaDecidere);
//   - nomenclatura_radice_non_verificata: modello nuovo, una gerarchia vuota si conferma solo dopo la nomenclatura della
//     radice (R80);
//   - fonte_non_confermata: la gerarchia sarebbe verificata, ma il prodotto non ha la fonte STEP confermata (K-02,
//     lettura A: T-E1R-04);
//   - bom_di_lavoro_assente: legacy, la gerarchia sarebbe verificata e la fonte è confermata, ma la struttura del
//     prodotto non è la BOM di lavoro dello STEP confermato (dubbio T-B5-17: gerarchiaSenzaBOMDiLavoro).
const (
	MotivoAsseConflitto             = "conflitto"
	MotivoAsseTargetSenzaComponente = "target_senza_componente"
	MotivoAsseSenzaFigli            = "senza_figli_nessun_gesto"
	MotivoAsseNessunGesto           = "nessun_gesto"
	MotivoAsseNessunaStruttura      = "nessuna_struttura"
	MotivoAsseDaDecidere            = "da_decidere"
	MotivoAsseNomenclaturaRadice    = "nomenclatura_radice_non_verificata"
	MotivoAsseFonteNonConfermata    = "fonte_non_confermata"
	MotivoAsseBOMDiLavoroAssente    = "bom_di_lavoro_assente"
)

// VerificaAsse: uno dei due assi (contratto §2.3, §2.6).
//   - Stato, Motivo: lo stato e, quando non è verificata, il primo motivo che vale (MotivoAsse*).
//   - Legacy: l'asse è letto dall'adattatore di «Conferma l'albero».
//   - DaDecidere: quanto resta da decidere. Legacy: le righe dei nodi della struttura senza la decisione di una persona,
//     esclusa la riga della radice (R80), e i nodi senza nessuna riga legacy (T-B5-15), più le relazioni confermate
//     della BOM che il segno non copre; per la gerarchia
//     anche le righe degli archi senza la decisione di una persona, e gli archi senza nessuna riga legacy (R-21). Modello
//     nuovo: i nodi (o gli archi) della struttura che il gesto non copre, più le relazioni confermate della BOM che il
//     gesto non copre (T-B5-99).
//   - Conflitti: i Rif degli oggetti in conflitto sull'asse (Conflitto.Rif), in ordine, senza doppioni.
type VerificaAsse struct {
	Stato      StatoVerificaAsse `json:"stato"`
	Legacy     bool              `json:"legacy"`
	DaDecidere int               `json:"da_decidere"`
	Conflitti  []string          `json:"conflitti,omitempty"`
	Motivo     string            `json:"motivo,omitempty"`
}

// VerificaBOM: la verifica della BOM di un prodotto (contratto §2.3; R80).
//   - Nomenclatura, Gerarchia: gli assi 3 e 4.
//   - Verificata: tutte e due verificata (R80): è NomenclaturaBOMVerificata AND GerarchiaBOMVerificata (§1.0).
//   - FonteRegistrata: la verifica è legata a una fonte STEP registrata (R61 A). Mai con il legacy: «Conferma l'albero»
//     conferma la BOM, non la fonte, e il suo segno non dice né STEP né sha256 né radice (A0.2 C9; diagnostica
//     bom.fonte_non_registrata quando il gesto legacy c'è). Nel modello nuovo vero con la fonte confermata, perché la
//     verifica si fa sulla BOM di lavoro, che esiste solo sotto la radice scelta dello STEP confermato (R76 A, R85;
//     dubbio T-B5-05).
//   - RimozioniAperte: le rimozioni proposte aperte sul perimetro del prodotto (ognuna è anche un conflitto di
//     gerarchia: T-B0-24).
//   - SenzaFigli: il prodotto non ha figli, né confermati né proposti (R80).
type VerificaBOM struct {
	Nomenclatura    VerificaAsse `json:"nomenclatura"`
	Gerarchia       VerificaAsse `json:"gerarchia"`
	Verificata      bool         `json:"verificata"`
	FonteRegistrata bool         `json:"fonte_registrata"`
	RimozioniAperte int          `json:"rimozioni_aperte"`
	SenzaFigli      bool         `json:"senza_figli"`
}

// StrutturaDaVerificare: la struttura di un prodotto come la vede la regola della verifica, l'altro ingresso astratto
// di VerificaDellaBOM (contratto §2.3: «VerificaBOM(GestiVerificaBOM, struttura)»; tipo della fase 1 di B5, dubbio
// T-B5-04). La prepara strutturaDaVerificare dalla fotografia e dalle strutture di ancoraggio; le prove la scrivono a mano.
//   - ConComponente: il target ha la sua riga componente (T-B0-07).
//   - FonteConfermata: la fonte STEP del prodotto è confermata (asse 2; K-02).
//   - BOMDiLavoro: le strutture che contano sono la BOM di lavoro dello STEP confermato (bom_di_lavoro_proposta: R76 A);
//     falso con la fonte confermata quando la BOM di lavoro non c'è (analisi in corso, radice non registrata, STEP
//     senza struttura letta, nessuna struttura: dubbio T-B5-17).
//   - Radici, Nodi, Archi: le radici e i nodi (RifNodo) e gli archi proposti (RifArco) delle strutture del prodotto che
//     contano: la BOM di lavoro, se c'è, altrimenti tutte le strutture candidate del prodotto (R71 A, R76 A).
//   - ArchiConfermati: le relazioni confermate della BOM confermata del prodotto (raggiungibili dal suo componente, fra
//     componenti non archiviati: R62 e A), come RifArco fra due RifComponente.
//   - RigheDaDecidere: le righe dei nodi di quelle strutture senza la decisione di una persona (aperte, o decise da un
//     automatismo: T-B4-24), esclusa la riga della radice della struttura (R80); RifRigaProposta. In più, con il loro
//     RifNodo, i nodi che non hanno nessuna riga legacy del loro contenuto su nessun allegato (dubbio T-B5-15).
//   - ArchiDaDecidere: le righe degli archi di quelle strutture senza la decisione di una persona (rigaArco). In più, con
//     il loro RifArco, gli archi che non hanno nessuna riga legacy del loro contenuto (sha256, chiavi del padre e del
//     figlio) su nessun allegato: la rete simmetrica a quella dei nodi (R-21 della revisione della fase 3).
//   - RimozioniAperte: le rimozioni proposte aperte sul perimetro del prodotto (RifArco fra due RifComponente).
//   - Conflitti: i conflitti di nomenclatura e di gerarchia del prodotto (ConflittiDellaNomenclatura e quelli della
//     gerarchia), che bloccano l'asse (R95 A).
type StrutturaDaVerificare struct {
	ConComponente   bool        `json:"con_componente"`
	FonteConfermata bool        `json:"fonte_confermata"`
	BOMDiLavoro     bool        `json:"bom_di_lavoro"`
	Radici          []string    `json:"radici,omitempty"`
	Nodi            []string    `json:"nodi,omitempty"`
	Archi           []string    `json:"archi,omitempty"`
	ArchiConfermati []string    `json:"archi_confermati,omitempty"`
	RigheDaDecidere []string    `json:"righe_da_decidere,omitempty"`
	ArchiDaDecidere []string    `json:"archi_da_decidere,omitempty"`
	RimozioniAperte []string    `json:"rimozioni_aperte,omitempty"`
	Conflitti       []Conflitto `json:"conflitti,omitempty"`
}

// PrefissoRifArco e RifArco: il riferimento di un arco, «arco:<padre>><figlio>», con i Rif dei due estremi (due nodi
// di una struttura, o due componenti per una relazione confermata).
const PrefissoRifArco = "arco:"

// RifArco: il riferimento dell'arco fra quei due estremi.
func RifArco(padre, figlio string) string { return PrefissoRifArco + padre + ">" + figlio }

// rigaArco: il riferimento di una riga di relazione_proposta, che non ha un ID: l'allegato e le due chiavi.
func rigaArco(allegato uuid.UUID, padre, figlio string) string {
	return "relazione_proposta:" + allegato.String() + ":" + padre + ">" + figlio
}

// VerificaDellaBOM: la regola della verifica della BOM (R80; contratto §1.0 righe 3 e 4, §1.3; T-B0-22). È pura: si
// prova con gesti e strutture sintetici (il prodotto senza figli del modello nuovo, PO-03), e sui dati veri la nutre
// l'adattatore di «Conferma l'albero». Per ognuno dei due assi vale il primo che si applica:
//  1. conflitto, se l'asse ha un conflitto (T-B0-24, R95 A): prevale su tutto, anche su non_verificabile, perché è una
//     decisione contraddetta che l'operatore deve risolvere (dubbio T-B5-09);
//  2. non_verificabile, per un target senza componente (T-B0-07), e nel legacy per un prodotto senza figli (R80);
//  3. da_verificare senza il gesto (nessun_gesto); nel modello nuovo anche senza struttura (nessuna_struttura);
//  4. da_verificare con qualcosa da decidere (da_decidere): nel legacy le righe senza la decisione di una persona,
//     esclusa la radice (R80, T-B4-24), e le relazioni confermate che il segno non copre (le decisioni prese una a una
//     non sono l'adattatore: R80, R71 B); nel modello nuovo i nodi o gli archi che il gesto non copre e, come nel legacy,
//     le relazioni confermate che il gesto non copre (T-B5-99: altrimenti un gesto sulla gerarchia vuota dello STEP
//     verificherebbe una BOM con figli confermati a mano);
//  5. per la gerarchia del modello nuovo, una gerarchia vuota resta da verificare finché la nomenclatura non è
//     verificata (R80: «si conferma dopo la nomenclatura della radice»). Vuota vuol dire senza archi nelle strutture e
//     senza relazioni confermate (T-B5-99; R80: «senza figli né confermati né proposti»);
//  6. altrimenti verificata.
//
// Poi, nel legacy con la fonte confermata ma senza la BOM di lavoro, la gerarchia non è verificata
// (gerarchiaSenzaBOMDiLavoro, dubbio T-B5-17); e la lettura A di K-02 (bomSenzaFonteConfermataK02): senza la fonte
// confermata la gerarchia non è verificata. Le due condizioni non si toccano: una vuole la fonte confermata, l'altra no.
func VerificaDellaBOM(g GestiVerificaBOM, s StrutturaDaVerificare) VerificaBOM {
	senzaFigli := len(s.ArchiConfermati) == 0 && len(differenza(s.Nodi, s.Radici)) == 0
	nom := VerificaAsse{Legacy: g.Legacy, Conflitti: rifDeiConflitti(s.Conflitti, AsseNomenclatura)}
	ger := VerificaAsse{Legacy: g.Legacy, Conflitti: rifDeiConflitti(s.Conflitti, AsseGerarchia)}
	if g.Legacy {
		nom.DaDecidere = len(s.RigheDaDecidere) + len(differenza(s.ArchiConfermati, archiDel(g.Nomenclatura)))
		ger.DaDecidere = len(s.RigheDaDecidere) + len(s.ArchiDaDecidere) + len(differenza(s.ArchiConfermati, archiDel(g.Gerarchia)))
	} else {
		nom.DaDecidere = len(differenza(s.Nodi, nodiDel(g.Nomenclatura))) + len(differenza(s.ArchiConfermati, archiDel(g.Nomenclatura)))
		ger.DaDecidere = len(differenza(s.Archi, archiDel(g.Gerarchia))) + len(differenza(s.ArchiConfermati, archiDel(g.Gerarchia)))
	}

	statoDi := func(a *VerificaAsse, gesto *GestoVerifica) {
		switch {
		case len(a.Conflitti) > 0:
			a.Stato, a.Motivo = StatoAsseConflitto, MotivoAsseConflitto
		case !s.ConComponente:
			a.Stato, a.Motivo = StatoAsseNonVerificabile, MotivoAsseTargetSenzaComponente
		case g.Legacy && senzaFigli:
			a.Stato, a.Motivo = StatoAsseNonVerificabile, MotivoAsseSenzaFigli
		case gesto == nil:
			a.Stato, a.Motivo = StatoAsseDaVerificare, MotivoAsseNessunGesto
		case !g.Legacy && len(s.Nodi) == 0:
			a.Stato, a.Motivo = StatoAsseDaVerificare, MotivoAsseNessunaStruttura
		case a.DaDecidere > 0:
			a.Stato, a.Motivo = StatoAsseDaVerificare, MotivoAsseDaDecidere
		default:
			a.Stato, a.Motivo = StatoAsseVerificata, ""
		}
	}
	statoDi(&nom, g.Nomenclatura)
	statoDi(&ger, g.Gerarchia)
	if !g.Legacy && ger.Stato == StatoAsseVerificata && len(s.Archi) == 0 && len(s.ArchiConfermati) == 0 && nom.Stato != StatoAsseVerificata {
		ger.Stato, ger.Motivo = StatoAsseDaVerificare, MotivoAsseNomenclaturaRadice
	}
	ger = gerarchiaSenzaBOMDiLavoro(ger, s.FonteConfermata, s.BOMDiLavoro)
	nom, ger = bomSenzaFonteConfermataK02(nom, ger, s.FonteConfermata)

	return VerificaBOM{Nomenclatura: nom, Gerarchia: ger,
		Verificata:      nom.Stato == StatoAsseVerificata && ger.Stato == StatoAsseVerificata,
		FonteRegistrata: !g.Legacy && s.FonteConfermata, RimozioniAperte: len(s.RimozioniAperte), SenzaFigli: senzaFigli}
}

// bomSenzaFonteConfermataK02: la BOM di un prodotto senza la fonte STEP confermata. È la lettura A di K-02, confermata
// dall'utente il 06/10 [U] (E1R §3.2): vale per l'adattatore legacy di «Conferma l'albero» e per il contratto A1c
// corrente; non stabilisce che lo STEP sia per sempre l'unica fonte possibile di evidenza BOM o gerarchica (T-E1R-04;
// contratto §1.0 riga 4 e §1.3, la nota su R61 A).
// Senza FonteSTEPConfermata la gerarchia non è mai verificata: dove sarebbe verificata (il gesto c'è, legacy o del
// modello nuovo, e niente resta da decidere) diventa da_verificare con il motivo fonte_non_confermata, e il gesto
// legacy resta visibile accanto (Legacy, DaDecidere). La nomenclatura non cambia: i codici si verificano anche prima
// (R81). non_verificabile prevale (nessun gesto da leggere), e così un conflitto o un altro motivo: fonte_non_confermata
// vale solo quando è l'unica cosa che manca, quindi una gerarchia ferma solo per lei dopo una fonte superata è ferma per
// una condizione nuova (T-E1-14). La lettura sta tutta in questa funzione: nient'altro dipende da K-02.
func bomSenzaFonteConfermataK02(nomenclatura, gerarchia VerificaAsse, fonteConfermata bool) (VerificaAsse, VerificaAsse) {
	if !fonteConfermata && gerarchia.Stato == StatoAsseVerificata {
		gerarchia.Stato, gerarchia.Motivo = StatoAsseDaVerificare, MotivoAsseFonteNonConfermata
	}
	return nomenclatura, gerarchia
}

// gerarchiaSenzaBOMDiLavoro: la gerarchia legacy di un prodotto con la fonte STEP confermata ma senza la BOM di lavoro
// (dubbio T-B5-17 [T], lettura prudente dell'orchestratore, da confermare dall'utente). La gerarchia dipende dalla BOM
// di lavoro, quindi dalla fonte (tabella degli assi dell'emendamento E1), e viene dallo STEP (E1R §3.2): con la fonte
// confermata ma la struttura che non è la BOM di lavoro (l'analisi in corso, la radice non registrata, lo STEP
// confermato senza una struttura letta, nessuna struttura) il segno di «Conferma l'albero» non basta, e la gerarchia
// che sarebbe verificata è da_verificare con il motivo bom_di_lavoro_assente. Vale solo per l'adattatore legacy: nel
// modello nuovo il gesto si fa sulla BOM di lavoro, e senza di lei non c'è niente da coprire. La nomenclatura non
// cambia; non_verificabile, un conflitto o un altro motivo prevalgono, come per K-02. Non è K-02 (che resta solo per la
// fonte non confermata, in bomSenzaFonteConfermataK02): è la stessa lettura per cui una BOM di lavoro assente non chiude
// il perimetro.
func gerarchiaSenzaBOMDiLavoro(gerarchia VerificaAsse, fonteConfermata, bomDiLavoro bool) VerificaAsse {
	if gerarchia.Legacy && fonteConfermata && !bomDiLavoro && gerarchia.Stato == StatoAsseVerificata {
		gerarchia.Stato, gerarchia.Motivo = StatoAsseDaVerificare, MotivoAsseBOMDiLavoroAssente
	}
	return gerarchia
}

// ---- l'adattatore legacy e la struttura del prodotto ----

// bomDelThread: ciò che serve alla verifica della BOM dei prodotti di un thread, calcolato una volta: i componenti
// attivi, le relazioni confermate fra loro per padre, i segni di «Conferma l'albero» sugli archi, le righe degli archi,
// le rimozioni aperte, i nodi (sha256, chiave) e gli archi (sha256, chiavi) che hanno almeno una riga legacy, gli archi
// tolti da una persona.
type bomDelThread struct {
	t           fotorfq.Thread
	figli       map[uuid.UUID][]fotorfq.Relazione
	relazioni   map[string]fotorfq.Relazione // per (padre, figlio)
	segni       []segnoArco
	shaAll      map[uuid.UUID]string
	nodiConRiga map[string]bool // RifNodo di ogni riga di componente_proposta, su qualunque allegato e in qualunque stato
	// archiConRiga: il RifArco (fra due RifNodo) di ogni riga di relazione_proposta, su qualunque allegato e in qualunque
	// stato; archiTolti: quelli le cui righe sono tutte scartate da una persona (l'arco tolto: dubbio T-B5-67).
	archiConRiga map[string]bool
	archiTolti   map[string]bool
}

// segnoArco: il segno di «Conferma l'albero» su una riga di arco tenuta: i due componenti del legame, chi e quando.
type segnoArco struct {
	padre, figlio uuid.UUID
	da            uuid.UUID
	il            string
}

func nuovaBOMDelThread(t fotorfq.Thread) *bomDelThread {
	b := &bomDelThread{t: t, figli: map[uuid.UUID][]fotorfq.Relazione{}, relazioni: map[string]fotorfq.Relazione{}, shaAll: map[uuid.UUID]string{},
		nodiConRiga: map[string]bool{}}
	for _, r := range t.RigheComponenteProposta {
		if r.Sha256 != "" && r.Chiave != "" {
			b.nodiConRiga[ancoraggio.RifNodo(r.Sha256, r.Chiave)] = true
		}
	}
	attivi := map[uuid.UUID]bool{}
	for _, k := range t.Componenti {
		if k.ArchiviatoIl == nil {
			attivi[k.ID] = true
		}
	}
	for _, r := range relazioniAttive(t.Relazioni, attivi) {
		b.figli[r.PadreID] = append(b.figli[r.PadreID], r)
		b.relazioni[r.PadreID.String()+"\x00"+r.FiglioID.String()] = r
	}
	for _, a := range t.Allegati {
		if a.Sha256 != nil {
			b.shaAll[a.ID] = *a.Sha256
		}
	}
	b.archiConRiga, b.archiTolti = map[string]bool{}, map[string]bool{}
	tenuti := map[string]bool{}
	for _, r := range t.RigheRelazioneProposta {
		sha := b.shaAll[r.AllegatoID]
		if sha == "" || r.PadreChiave == "" || r.FiglioChiave == "" {
			continue
		}
		k := RifArco(ancoraggio.RifNodo(sha, r.PadreChiave), ancoraggio.RifNodo(sha, r.FiglioChiave))
		b.archiConRiga[k] = true
		if r.Stato == statoPropostaScartata && r.DecisoDa != nil {
			b.archiTolti[k] = true
		} else {
			tenuti[k] = true
		}
	}
	for k := range tenuti {
		delete(b.archiTolti, k)
	}
	// I segni sugli archi: le righe di relazione_proposta tenute dalla conferma dell'albero, cioè confermate o duplicato,
	// decise da una persona, con i due componenti nel segno (la forma di fascicolo.ArcoConfermatoNellAlbero). Le righe
	// scartate con il segno sono archi tolti: contano come decise (escono dalle righe da decidere), non come archi del
	// gesto.
	for _, r := range t.RigheRelazioneProposta {
		a := r.Albero
		if a == nil || r.DecisoDa == nil || (r.Stato != statoPropostaConfermata && r.Stato != statoPropostaDuplicato) || a.Padre == nil || a.Figlio == nil {
			continue
		}
		b.segni = append(b.segni, segnoArco{padre: *a.Padre, figlio: *a.Figlio, da: a.Da, il: a.Il})
	}
	return b
}

// perimetro: il componente del prodotto e i componenti attivi raggiungibili da lui per le relazioni confermate (la BOM
// confermata: R62 e A), con le relazioni percorse; vuoto senza componente.
func (b *bomDelThread) perimetro(componente *uuid.UUID) (map[uuid.UUID]bool, []fotorfq.Relazione) {
	dentro := map[uuid.UUID]bool{}
	if componente == nil {
		return dentro, nil
	}
	dentro[*componente] = true
	coda := []uuid.UUID{*componente}
	var archi []fotorfq.Relazione
	for len(coda) > 0 {
		p := coda[0]
		coda = coda[1:]
		for _, r := range b.figli[p] {
			archi = append(archi, r)
			if !dentro[r.FiglioID] {
				dentro[r.FiglioID] = true
				coda = append(coda, r.FiglioID)
			}
		}
	}
	return dentro, archi
}

// gestiDaConfermaLAlbero: l'adattatore legacy di «Conferma l'albero» (R80, R61 A; contratto §1.3; T-B0-22). Il gesto
// è il segno sugli archi del prodotto: le righe di arco tenute dalla conferma (confermate o duplicato, decise da una
// persona, con padre e figlio nel segno) il cui padre è nel perimetro del prodotto. Lo stesso gesto vale per
// nomenclatura e gerarchia: la conferma ha confermato codici e albero insieme (R80). Copre le relazioni dei segni
// (RifArco fra due RifComponente) e i loro componenti; chi e quando sono quelli del segno più recente (il più grande
// «il», a parità il «da» minore: dubbio T-B5-10). Senza un segno nel perimetro, o senza il componente del target,
// nessun gesto (Legacy resta vero). Se il gesto copre tutta la BOM, e niente resta da decidere, lo dice la regola.
func (b *bomDelThread) gestiDaConfermaLAlbero(pv ProdottoValutato) GestiVerificaBOM {
	g := GestiVerificaBOM{Legacy: true}
	perimetro, _ := b.perimetro(pv.ComponenteID)
	var archi, nodi []string
	var ultimo *segnoArco
	for i := range b.segni {
		s := &b.segni[i]
		if !perimetro[s.padre] {
			continue
		}
		archi = append(archi, RifArco(ancoraggio.RifComponente(s.padre), ancoraggio.RifComponente(s.figlio)))
		nodi = append(nodi, ancoraggio.RifComponente(s.padre), ancoraggio.RifComponente(s.figlio))
		if ultimo == nil || s.il > ultimo.il || (s.il == ultimo.il && s.da.String() < ultimo.da.String()) {
			ultimo = s
		}
	}
	if ultimo == nil {
		return g
	}
	da := ultimo.da
	gesto := GestoVerifica{Da: &da, Il: ultimo.il, Nodi: ordinatiUnici(nodi), Archi: ordinatiUnici(archi)}
	altro := gesto
	altro.Da = copiaUUID(gesto.Da)
	altro.Nodi, altro.Archi = append([]string(nil), gesto.Nodi...), append([]string(nil), gesto.Archi...)
	g.Nomenclatura, g.Gerarchia = &gesto, &altro
	return g
}

// strutturaDaVerificare: la struttura del prodotto per la regola (StrutturaDaVerificare), dalla fotografia e dalle
// strutture di ancoraggio:
//   - le strutture che contano (struttureDellaVerifica): la BOM di lavoro se c'è, altrimenti le strutture candidate del
//     prodotto (R71 A, R76 A);
//   - le righe da decidere: le righe aperte della struttura (StrutturaProdotto.RigheDaDecidere, che lascia fuori la riga
//     della radice della struttura: R80, anche per un prodotto che è un nodo interno di uno STEP più grande) e le righe
//     decise da un automatismo, senza chi le ha decise (NodoProposto.DecisoDaPersona falso: un duplicato automatico non
//     vale come la decisione di una persona, come per il gate legacy; T-B4-24, dubbio T-B5-11), radice esclusa;
//   - la rete di sicurezza (dubbio T-B5-15): un nodo non radice che non ha nessuna riga legacy del suo contenuto
//     (sha256, chiave) su nessun allegato conta fra i da decidere, con il suo RifNodo: il gesto legacy non può coprire
//     un nodo che il legacy non ha mai proposto;
//   - le righe degli archi della struttura (padre fra i nodi della struttura, nel suo allegato e nel suo contenuto) aperte
//     o decise da un automatismo; quelle decise da una persona, anche scartate, sono decise;
//   - la rete degli archi (R-21 della revisione della fase 3, simmetrica a quella dei nodi): un arco della struttura che
//     non ha nessuna riga legacy del suo contenuto (sha256, chiavi del padre e del figlio) su nessun allegato conta fra
//     gli archi da decidere, con il suo RifArco: nessuno l'ha deciso, e il perimetro non si chiude (§1.6);
//   - le relazioni confermate e le rimozioni aperte del perimetro;
//   - se le strutture che contano sono la BOM di lavoro (BOMDiLavoro, per T-B5-17);
//   - i conflitti di nomenclatura (ConflittiDellaNomenclatura) e di gerarchia (conflittiDellaGerarchia).
func (b *bomDelThread) strutturaDaVerificare(pv ProdottoValutato, strutture []ancoraggio.StrutturaProdotto) StrutturaDaVerificare {
	s := StrutturaDaVerificare{ConComponente: pv.ComponenteID != nil, FonteConfermata: pv.Fonte.Calcolata && pv.Fonte.Stato == FonteConfermata}
	scelte := struttureDellaVerifica(strutture, pv.Rif)
	var radici, nodi, archi, righe, righeArchi []string
	for _, st := range scelte {
		if st.Stato == ancoraggio.StatoBOMDiLavoroProposta {
			s.BOMDiLavoro = true
		}
		radici = append(radici, st.Radice)
		dentro := map[string]bool{}
		for _, n := range st.Nodi {
			nodi = append(nodi, n.Rif)
			dentro[n.Rif] = true
			if n.Rif != st.Radice && n.RigaDecisa != nil && !n.DecisoDaPersona {
				righe = append(righe, ancoraggio.RifRigaProposta(n.RigaDecisa.ID))
			}
			if n.Rif != st.Radice && !b.nodiConRiga[n.Rif] {
				righe = append(righe, n.Rif)
			}
		}
		for _, a := range st.Archi {
			archi = append(archi, RifArco(a.Padre, a.Figlio))
			if !b.archiConRiga[RifArco(a.Padre, a.Figlio)] {
				righeArchi = append(righeArchi, RifArco(a.Padre, a.Figlio))
			}
		}
		righe = append(righe, st.RigheDaDecidere...)
		for _, r := range b.t.RigheRelazioneProposta {
			if r.AllegatoID != st.AllegatoID || b.shaAll[r.AllegatoID] != st.Sha256 || !dentro[ancoraggio.RifNodo(st.Sha256, r.PadreChiave)] {
				continue
			}
			if r.Stato == statoPropostaAperta || r.DecisoDa == nil {
				righeArchi = append(righeArchi, rigaArco(r.AllegatoID, r.PadreChiave, r.FiglioChiave))
			}
		}
	}
	s.Radici, s.Nodi, s.Archi = ordinatiUnici(radici), ordinatiUnici(nodi), ordinatiUnici(archi)
	s.RigheDaDecidere, s.ArchiDaDecidere = ordinatiUnici(righe), ordinatiUnici(righeArchi)

	perimetro, relazioni := b.perimetro(pv.ComponenteID)
	var confermati []string
	for _, r := range relazioni {
		confermati = append(confermati, RifArco(ancoraggio.RifComponente(r.PadreID), ancoraggio.RifComponente(r.FiglioID)))
	}
	s.ArchiConfermati = ordinatiUnici(confermati)
	var rimozioni []fotorfq.RimozioneAperta
	var rifRimozioni []string
	for _, r := range b.t.RimozioniAperte {
		if perimetro[r.PadreID] {
			rimozioni = append(rimozioni, r)
			rifRimozioni = append(rifRimozioni, RifArco(ancoraggio.RifComponente(r.PadreID), ancoraggio.RifComponente(r.FiglioID)))
		}
	}
	s.RimozioniAperte = ordinatiUnici(rifRimozioni)
	s.Conflitti = append(ConflittiDellaNomenclatura(pv.Rif, strutture), b.conflittiDellaGerarchia(pv.Rif, scelte, rimozioni)...)
	return s
}

// struttureDellaVerifica: le strutture di un prodotto che contano per la verifica (R71 A, R76 A): la BOM di lavoro, se
// c'è (al più una per prodotto), altrimenti tutte le strutture candidate del prodotto, nell'ordine di ancoraggio.
func struttureDellaVerifica(strutture []ancoraggio.StrutturaProdotto, prodotto string) []ancoraggio.StrutturaProdotto {
	var candidate, bom []ancoraggio.StrutturaProdotto
	for _, s := range strutture {
		switch {
		case s.Target != prodotto:
		case s.Stato == ancoraggio.StatoBOMDiLavoroProposta:
			bom = append(bom, s)
		default:
			candidate = append(candidate, s)
		}
	}
	if len(bom) > 0 {
		return bom
	}
	return candidate
}

// diagnosticaFonteNonRegistrata: bom.fonte_non_registrata per un prodotto la cui BOM è letta dal gesto legacy (R61 A).
func diagnosticaFonteNonRegistrata(pv ProdottoValutato) evidenze.Diagnostica {
	return evidenze.Diagnostica{
		Codice:    CodiceBOMFonteNonRegistrata,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaDati,
		Percorso:  "prodotti[" + pv.Rif + "].bom",
		Messaggio: "la BOM è letta da «Conferma l'albero», che conferma la BOM ma non registra la fonte (né STEP, né sha256, né radice): la fonte resta quella dell'asse 2 (R61 A)",
		Rif:       []string{pv.Rif},
	}
}

// ---- gli insiemi ----

// differenza: gli elementi di a che non sono in b, senza doppioni.
func differenza(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, x := range b {
		in[x] = true
	}
	var out []string
	visti := map[string]bool{}
	for _, x := range a {
		if !in[x] && !visti[x] {
			visti[x] = true
			out = append(out, x)
		}
	}
	return out
}

func nodiDel(g *GestoVerifica) []string {
	if g == nil {
		return nil
	}
	return g.Nodi
}

func archiDel(g *GestoVerifica) []string {
	if g == nil {
		return nil
	}
	return g.Archi
}

// ordinatiUnici: una copia in ordine, senza doppioni; nil se vuota.
func ordinatiUnici(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := append([]string(nil), s...)
	sort.Strings(out)
	j := 0
	for i, x := range out {
		if i == 0 || x != out[j-1] {
			out[j] = x
			j++
		}
	}
	return out[:j]
}
