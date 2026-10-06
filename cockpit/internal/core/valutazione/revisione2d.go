package valutazione

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// La revisione del documento (B5, fase 2; R104 [U]; E1R §5; contratto §1.6, §2.6; T-E1R-05…T-E1R-09): evidenza letta →
// revisione proposta → revisione confermata. La proposta segue cartiglio leggibile → STEP dell'entità → nome del file,
// mai la formazione grezza, mai propagata ai figli; documento.rev è un valore registrato, con la provenienza, fuori
// dalla proposta e mai un conflitto; la decisione arriva solo da una DecisioneIdentita sul documento (in A1c nessun
// adattatore: LD-27), e il ricalcolo la conserva e le mette accanto le evidenze nuove contrarie (il conflitto
// identita_documento, come pezzo); le discordanze si calcolano sempre e niente le nasconde.

// ---- i tipi del recepimento E1R (contratto §2.6) ----

// Le fonti di un'evidenza di revisione (EvidenzaRevisione.Fonte) e della proposta (RevisioneProposta.Fonte): le stesse
// parole delle evidenze viste di ancoraggio (EvidenzaVista.Fonte, T-B4-22), più «assente» per la proposta.
const (
	FonteEvidenzaCartiglio  = ancoraggio.FonteEvidenzaCartiglio
	FonteEvidenzaStepEntita = ancoraggio.FonteEvidenzaStepEntita
	FonteEvidenzaNomeFile   = ancoraggio.FonteEvidenzaNomeFile
	FonteEvidenzaDocumento  = ancoraggio.FonteEvidenzaDocumento
	FontePropostaAssente    = "assente"
)

// EvidenzaRevisione: un valore di revisione trovato in una fonte, con la sua provenienza (contratto §2.6; E1R §5.1).
//   - Fonte: cartiglio, step_entita, nome_file, documento.
//   - Valore: la revisione com'è letta (la normalizzata della grammatica; per documento il valore registrato senza gli
//     spazi ai bordi); con Interpretabile falso, il testo grezzo che non si interpreta, se ce n'è uno; nil se la fonte
//     non porta una revisione.
//   - Entita: per step_entita il Rif del nodo dell'entità.
//   - Interpretabile: il valore si può usare (una fonte non interpretabile non inventa niente: E1R §5.2).
//   - Motivo: perché la fonte non dà una revisione interpretabile (MotivoEvidenza*); "" quando la dà.
type EvidenzaRevisione struct {
	Fonte          string  `json:"fonte"`
	Valore         *string `json:"valore,omitempty"`
	Entita         string  `json:"entita,omitempty"`
	Interpretabile bool    `json:"interpretabile"`
	Motivo         string  `json:"motivo,omitempty"`
}

// I motivi di un'evidenza di revisione (EvidenzaRevisione.Motivo; il contratto non li fissa: scelte della fase 2,
// dubbio T-B5-32):
//   - cartiglio: cartiglio_non_leggibile (un PDF senza cartiglio leggibile), formato_senza_lettura (un formato di cui il
//     cartiglio oggi non si legge: LD-04), revisione_assente_nel_cartiglio (il cartiglio si legge ma non porta una
//     revisione: decisioni dell'orchestratore, punto 5);
//   - step_entita: entita_assente (il 2D non ha un nodo), entita_non_univoca (più candidati, o più nodi di revisione
//     diversa: T-E1R-05), revisione_non_determinata (il nodo c'è, la sua identità non ha la revisione);
//   - nome_file: nome_non_letto (la grammatica non legge il nome), revisione_assente_nel_nome;
//   - per tutte: revisione_non_interpretabile (la revisione c'è ma non si interpreta: un token sospeso, una regola che
//     non la legge), letture_discordanti (le letture della fonte danno revisioni o identità diverse: nessuna si
//     sceglie, T-E1-07);
//   - documento: revisione_non_registrata (documento.rev vuoto).
const (
	MotivoEvidenzaCartiglioNonLeggibile      = "cartiglio_non_leggibile"
	MotivoEvidenzaFormatoSenzaLettura        = "formato_senza_lettura"
	MotivoEvidenzaRevisioneAssenteCartiglio  = "revisione_assente_nel_cartiglio"
	MotivoEvidenzaEntitaAssente              = "entita_assente"
	MotivoEvidenzaEntitaNonUnivoca           = "entita_non_univoca"
	MotivoEvidenzaRevisioneNonDeterminata    = "revisione_non_determinata"
	MotivoEvidenzaNomeNonLetto               = "nome_non_letto"
	MotivoEvidenzaRevisioneAssenteNome       = "revisione_assente_nel_nome"
	MotivoEvidenzaRevisioneNonInterpretabile = "revisione_non_interpretabile"
	MotivoEvidenzaLettureDiscordanti         = "letture_discordanti"
	MotivoEvidenzaRevisioneNonRegistrata     = "revisione_non_registrata"
)

// RevisioneProposta: il valore che il sistema propone per il documento (contratto §2.6; E1R §5.2), concettualmente
// documento_proposta.rev. Fonte: cartiglio, step_entita, nome_file o assente. Entita: per step_entita il Rif del nodo.
// Motivo: perché la proposta non viene dalla fonte prima (PropostaDiRevisione); per una proposta assente il motivo del
// nome del file (nome_non_interpretabile) o nessuna_fonte.
type RevisioneProposta struct {
	Valore *string `json:"valore,omitempty"`
	Fonte  string  `json:"fonte"`
	Entita string  `json:"entita,omitempty"`
	Motivo string  `json:"motivo,omitempty"`
}

// I valori di RevisioneProposta.Motivo (contratto §2.6).
const (
	MotivoPropostaCartiglioNonLeggibile   = "cartiglio_non_leggibile"
	MotivoPropostaRevisioneNonDeterminata = "revisione_non_determinata"
	MotivoPropostaEntitaNonUnivoca        = "entita_non_univoca"
	MotivoPropostaNomeNonInterpretabile   = "nome_non_interpretabile"
	MotivoPropostaNessunaFonte            = "nessuna_fonte"
)

// DiscordanzaRevisione: due revisioni diverse, con le due fonti (contratto §2.6; E1R §5.4; T-E1R-09). Tra: le due
// revisioni confrontate; A, B: i due valori (B vuoto: la decisione senza revisione); FonteA, FonteB: da dove vengono;
// Effetto: indicatore o conflitto; CalcolataDa: go, o vista per la rev_diversa di v_fascicolo. Nessun campo la nasconde.
type DiscordanzaRevisione struct {
	Tra         string `json:"tra"`
	A           string `json:"a"`
	B           string `json:"b"`
	FonteA      string `json:"fonte_a"`
	FonteB      string `json:"fonte_b"`
	Effetto     string `json:"effetto"`
	CalcolataDa string `json:"calcolata_da"`
}

// I valori di DiscordanzaRevisione (contratto §2.6). Le fonti del lato del componente in documento_componente:
// decisione (una decisione sul componente), step_entita (l'identità proposta dal nodo del componente), componente
// (componente.rev registrata).
const (
	TraDocumentoComponente = "documento_componente"
	TraPropostaRegistrata  = "proposta_registrata"
	TraPropostaConfermata  = "proposta_confermata"

	EffettoIndicatore = "indicatore"
	EffettoConflitto  = "conflitto"

	CalcolataDaGo    = "go"
	CalcolataDaVista = "vista"

	FonteComponenteDecisione   = "decisione"
	FonteComponenteStepEntita  = "step_entita"
	FonteComponenteRegistrata  = "componente"
	FonteDiscordanzaConfermata = "confermata"
)

// I valori di Disegno2D.StatoRevisione (contratto §2.6; E1R §5.1): il livello più alto che c'è.
const (
	StatoRevisione2DAssente    = "assente"
	StatoRevisione2DProposta   = "proposta"
	StatoRevisione2DRegistrata = "registrata"
	StatoRevisione2DConfermata = "confermata"
)

// ---- gli ingressi astratti della regola ----

// FonteDelDisegno: una fonte d'identità di un 2D come la legge l'adattatore (tipo della fase 2, dubbio T-B5-31): la
// revisione letta (Revisione, che va in Disegno2D.EvidenzeRevisione) e, per il codice della compatibilità e per il
// conflitto, il codice letto, la coppia fonte-valore e la provenienza.
//   - Coppia: la coppia (fonte, testo grezzo) costruita con ancoraggio.EvidenzaDa (T-B4-22): il testo di
//     cartiglio.codice, il grezzo dell'id del nodo, il nome del file, «codice rev» del documento. Vuota se la fonte non
//     ha un testo (un cartiglio che non si legge, nessun nodo).
//   - Codice, Lettura: il codice com'è nella fonte e la sua lettura con la grammatica del cliente (namespace, base,
//     marcatore); "" e nil se la fonte non dà un codice (anche per lo STEP dell'entità, che non è un codice del 2D:
//     T-E1R-06).
//   - AllegatoID, UnitaID, Posizione: da dove viene, per le evidenze del conflitto (T-E1-15).
type FonteDelDisegno struct {
	Revisione  EvidenzaRevisione        `json:"revisione"`
	Coppia     ancoraggio.EvidenzaVista `json:"coppia"`
	Codice     string                   `json:"codice,omitempty"`
	Lettura    *motorea.LetturaForma    `json:"lettura,omitempty"`
	AllegatoID *uuid.UUID               `json:"allegato_id,omitempty"`
	UnitaID    string                   `json:"unita_id,omitempty"`
	Posizione  evidenze.Localizzatore   `json:"posizione,omitzero"`
}

// DisegnoDaValutare: un 2D di un componente come lo legge l'adattatore, l'ingresso astratto di ValutaDisegno (tipo
// della fase 2, dubbio T-B5-31; T-B0-22).
//   - Disegno: i campi del file e del contenuto (documento, allegato, sha256, nome, formato, estensione, validità,
//     testo, cartiglio, provenienza, corrente, confermato il, anteprima) e la revisione registrata (Registrata,
//     RevProvenienza); il resto lo calcola la regola.
//   - Fonti: le fonti d'identità, al più una per fonte (cartiglio, step_entita, nome_file, documento).
//   - DiscordanzaVista: la rev_diversa della vista sul documento, come discordanza con CalcolataDa = vista (T-E1R-09).
//   - Decisione: la decisione del modello nuovo sul documento (ancoraggio.DecisioneIdentita con l'oggetto documento e
//     l'ID del documento; altrimenti non conta). In A1c nessun adattatore la produce (LD-27): la regola si prova con
//     decisioni sintetiche. LetturaDecisa: il codice deciso letto con la grammatica del cliente, per la parte codice
//     del conflitto; nil se non si legge (allora conta solo la revisione).
type DisegnoDaValutare struct {
	Disegno          Disegno2D                     `json:"disegno"`
	Fonti            []FonteDelDisegno             `json:"fonti,omitempty"`
	DiscordanzaVista *DiscordanzaRevisione         `json:"discordanza_vista,omitempty"`
	Decisione        *ancoraggio.DecisioneIdentita `json:"decisione,omitempty"`
	LetturaDecisa    *motorea.LetturaForma         `json:"lettura_decisa,omitempty"`
}

// ComponenteDaConfrontare: il lato del componente nel criterio 2 del primario e in documento_componente (tipo della
// fase 2, dubbio T-B5-31; T-B0-34, E1R §5.6). Lettura: il codice deciso del componente letto con la grammatica (R31 c);
// nil se non si legge. Revisione, FonteRevisione, Confronto: la revisione del componente, il primo che c'è: una
// decisione sul componente (decisione, deciso), l'identità proposta dal suo nodo dello STEP (step_entita, proposto),
// componente.rev come ultima risorsa (componente, deciso); nil, "" e nessuno senza.
type ComponenteDaConfrontare struct {
	ComponenteID   uuid.UUID             `json:"componente_id"`
	Lettura        *motorea.LetturaForma `json:"lettura,omitempty"`
	Revisione      *string               `json:"revisione,omitempty"`
	FonteRevisione string                `json:"fonte_revisione,omitempty"`
	Confronto      string                `json:"confronto"`
}

// RifDocumento: il riferimento di un documento confermato, l'oggetto del conflitto identita_documento.
func RifDocumento(id uuid.UUID) string { return prefissoRifDocumento + id.String() }

const prefissoRifDocumento = "documento:"

// ---- le regole ----

// ordineFonti: l'ordine delle fonti nelle evidenze e nella proposta (E1R §5.2), documento per ultimo (fuori dalla
// proposta: T-E1R-07).
var ordineFonti = []string{FonteEvidenzaCartiglio, FonteEvidenzaStepEntita, FonteEvidenzaNomeFile, FonteEvidenzaDocumento}

// PropostaDiRevisione: la revisione proposta per un documento dalle sue evidenze (E1R §5.2; T-E1R-05, T-E1R-07), la
// prima fonte con un valore interpretabile:
//  1. il cartiglio leggibile → cartiglio;
//  2. altrimenti lo STEP dell'entità (la revisione interpretata del nodo, IdentitaNodo.Revisione, mai rev_grezza) →
//     step_entita, con il motivo cartiglio_non_leggibile;
//  3. altrimenti il nome del file → nome_file, con il motivo dello STEP (entita_non_univoca, altrimenti
//     revisione_non_determinata);
//  4. altrimenti assente, con il motivo del nome: nome_non_interpretabile se la grammatica legge il nome ma non ne dà
//     una revisione interpretabile, altrimenti nessuna_fonte (nessuna fonte interpretabile).
//
// Il motivo dice perché non vale la fonte prima («si passa alla fonte dopo, con il motivo»: E1R §5.2); le evidenze
// dicono il perché di ognuna (dubbio T-B5-32). documento.rev non entra mai, nemmeno come quarta fonte (T-E1R-07). È
// pura: una fonte che manca dall'elenco vale come una fonte senza valore.
func PropostaDiRevisione(ev []EvidenzaRevisione) RevisioneProposta {
	per := map[string]EvidenzaRevisione{}
	for i := len(ev) - 1; i >= 0; i-- { // la prima evidenza di ogni fonte
		per[ev[i].Fonte] = ev[i]
	}
	motivo := ""
	for _, f := range ordineFonti[:3] {
		e, ok := per[f]
		if ok && e.Interpretabile && e.Valore != nil {
			p := RevisioneProposta{Valore: copiaTesto(e.Valore), Fonte: f, Motivo: motivo}
			if f == FonteEvidenzaStepEntita {
				p.Entita = e.Entita
			}
			return p
		}
		switch {
		case f == FonteEvidenzaCartiglio:
			motivo = MotivoPropostaCartiglioNonLeggibile
		case f == FonteEvidenzaStepEntita && ok && e.Motivo == MotivoEvidenzaEntitaNonUnivoca:
			motivo = MotivoPropostaEntitaNonUnivoca
		case f == FonteEvidenzaStepEntita:
			motivo = MotivoPropostaRevisioneNonDeterminata
		case ok && e.Motivo != MotivoEvidenzaNomeNonLetto:
			motivo = MotivoPropostaNomeNonInterpretabile
		default:
			motivo = MotivoPropostaNessunaFonte
		}
	}
	return RevisioneProposta{Fonte: FontePropostaAssente, Motivo: motivo}
}

// ValutaDisegno: la regola della revisione e dell'identità di un 2D di un componente (R104, E1R §5; T-E1R-05…09;
// T-B0-34, R84 criterio 2), su ingressi astratti (T-B0-22):
//  1. le evidenze lette, una per fonte, in ordine (cartiglio, step_entita, nome_file, documento);
//  2. la proposta (PropostaDiRevisione);
//  3. la decisione sul documento, se c'è: Confermata e CodiceConfermato (T-E1R-08);
//  4. lo stato: confermata (la decisione), poi registrata (documento.rev), poi proposta, poi assente;
//  5. l'identità della compatibilità (T-E1R-06): la revisione confermata, altrimenti il cartiglio, il nome del file,
//     documento.rev come ultima risorsa, mai lo STEP; il codice nella stessa precedenza; la compatibilità con il
//     componente: il codice con motorea.ConfrontaBasi e la regola unica del marcatore (T-B4-30), la revisione con
//     stessaRevisione; non_determinabile quando manca un lato (T-B0-34);
//  6. le discordanze (T-E1R-09), sempre: documento_componente con la revisione della compatibilità contro quella del
//     componente (mai la proposta dallo STEP, che coinciderebbe per costruzione), indicatore; quella della vista;
//     proposta_registrata, indicatore (documento.rev non è mai un conflitto: T-E1R-07); proposta_confermata, conflitto
//     se la fonte della proposta porta un'evidenza nuova, altrimenti indicatore;
//  7. i conflitti identita_documento (conflittiIdentitaDocumento), uno per ogni prodotto dato.
//
// prodotti: i prodotti a cui il documento è pertinente, che il chiamante conosce (B6: la pertinenza); senza, il
// conflitto esce con il prodotto vuoto, e lo attribuisce chi compone. È pura; l'ingresso non cambia.
func ValutaDisegno(d DisegnoDaValutare, c ComponenteDaConfrontare, prodotti []string) (Disegno2D, []Conflitto) {
	out := copiaDisegno(d.Disegno)
	fonti := fontiInOrdine(d.Fonti)
	out.EvidenzeRevisione = nil
	for _, f := range fonti {
		e := f.Revisione
		e.Valore = copiaTesto(e.Valore)
		out.EvidenzeRevisione = append(out.EvidenzeRevisione, e)
	}
	out.RevisioneProposta = PropostaDiRevisione(out.EvidenzeRevisione)

	dec := decisioneDelDocumento(d.Decisione, out.DocumentoID)
	out.Confermata, out.CodiceConfermato = nil, nil
	if dec != nil {
		codice := strings.TrimSpace(dec.Codice)
		out.CodiceConfermato = &codice
		if rev := strings.TrimSpace(dec.Revisione); rev != "" {
			out.Confermata = &rev
		}
	}
	switch {
	case dec != nil:
		out.StatoRevisione = StatoRevisione2DConfermata
	case out.Registrata != nil:
		out.StatoRevisione = StatoRevisione2DRegistrata
	case out.RevisioneProposta.Valore != nil:
		out.StatoRevisione = StatoRevisione2DProposta
	default:
		out.StatoRevisione = StatoRevisione2DAssente
	}

	lettura := identitaDellaCompatibilita(&out, fonti, dec, d.LetturaDecisa)
	out.CompatibilitaCodice = compatibilitaCodici(lettura, c.Lettura)
	out.CompatibilitaRevisione = motorea.CompatibilitaNonDeterminabile
	if out.Revisione != nil && c.Revisione != nil {
		out.CompatibilitaRevisione = motorea.CompatibilitaDiscordante
		if stessaRevisione(*out.Revisione, *c.Revisione) {
			out.CompatibilitaRevisione = motorea.CompatibilitaUguale
		}
	}
	out.ConfrontatoCon = ConfrontatoConNessuno
	if c.Revisione != nil && c.Confronto != "" {
		out.ConfrontatoCon = c.Confronto
	}

	out.Discordanze = discordanze(out, c, fonti, dec, d.DiscordanzaVista)
	return out, conflittiIdentitaDocumento(out, fonti, dec, d.LetturaDecisa, prodotti)
}

// fontiInOrdine: le fonti nell'ordine di ordineFonti, la prima per fonte; una fonte fuori elenco non entra.
func fontiInOrdine(fonti []FonteDelDisegno) []FonteDelDisegno {
	var out []FonteDelDisegno
	for _, f := range ordineFonti {
		for _, x := range fonti {
			if x.Revisione.Fonte == f {
				out = append(out, x)
				break
			}
		}
	}
	return out
}

// fonteDi: la fonte con quel nome fra le fonti in ordine; nil se non c'è.
func fonteDi(fonti []FonteDelDisegno, nome string) *FonteDelDisegno {
	for i := range fonti {
		if fonti[i].Revisione.Fonte == nome {
			return &fonti[i]
		}
	}
	return nil
}

// decisioneDelDocumento: la decisione, se è una decisione sul documento del 2D (l'oggetto documento e lo stesso ID);
// altrimenti nil: una decisione su un componente o su un altro documento non vale qui.
func decisioneDelDocumento(d *ancoraggio.DecisioneIdentita, documento *uuid.UUID) *ancoraggio.DecisioneIdentita {
	if d == nil || d.Oggetto != ancoraggio.OggettoDecisioneDocumento || documento == nil || d.ID != *documento {
		return nil
	}
	return d
}

// revisioneUsabile: il valore di un'evidenza interpretabile, senza gli spazi ai bordi; "" altrimenti.
func revisioneUsabile(e EvidenzaRevisione) string {
	if !e.Interpretabile || e.Valore == nil {
		return ""
	}
	return strings.TrimSpace(*e.Valore)
}

// identitaDellaCompatibilita: l'identità del 2D per il criterio 2 (T-E1R-06, E1R §5.6). La revisione (Revisione,
// FonteIdentita): la decisione sul documento; altrimenti il cartiglio, poi il nome del file, con un valore
// interpretabile; documento.rev come ultima risorsa (la registrata); mai lo STEP dell'entità. Il codice (Codice e la
// sua lettura, che restituisce): il codice deciso, altrimenti il primo fra cartiglio, nome del file e documento che ne
// dà uno.
func identitaDellaCompatibilita(out *Disegno2D, fonti []FonteDelDisegno, dec *ancoraggio.DecisioneIdentita, letturaDecisa *motorea.LetturaForma) *motorea.LetturaForma {
	out.Codice, out.Revisione, out.FonteIdentita = "", nil, ""
	if dec != nil {
		out.Codice, out.Revisione, out.FonteIdentita = *out.CodiceConfermato, copiaTesto(out.Confermata), FonteIdentitaConfermata
		return letturaDecisa
	}
	for _, f := range []string{FonteEvidenzaCartiglio, FonteEvidenzaNomeFile} {
		if x := fonteDi(fonti, f); x != nil && revisioneUsabile(x.Revisione) != "" {
			v := revisioneUsabile(x.Revisione)
			out.Revisione, out.FonteIdentita = &v, f
			break
		}
	}
	if out.Revisione == nil && out.Registrata != nil {
		v := strings.TrimSpace(*out.Registrata)
		out.Revisione, out.FonteIdentita = &v, FonteIdentitaDocumento
	}
	for _, f := range []string{FonteEvidenzaCartiglio, FonteEvidenzaNomeFile, FonteEvidenzaDocumento} {
		if x := fonteDi(fonti, f); x != nil && strings.TrimSpace(x.Codice) != "" {
			out.Codice = strings.TrimSpace(x.Codice)
			return x.Lettura
		}
	}
	return nil
}

// compatibilitaCodici: il codice del 2D contro quello del componente (R84 criterio 2): lo stesso namespace, poi
// motorea.ConfrontaBasi con la regola unica del marcatore (T-B4-30: due marcatori scritti e diversi sono un'altra
// identità); non_determinabile senza uno dei due o con due namespace diversi.
func compatibilitaCodici(a, b *motorea.LetturaForma) motorea.Compatibilita {
	if a == nil || b == nil || a.Namespace != b.Namespace {
		return motorea.CompatibilitaNonDeterminabile
	}
	c := motorea.ConfrontaBasi(a.Base, b.Base)
	ma, mb := marcatoreDiForma(a), marcatoreDiForma(b)
	if (c == motorea.CompatibilitaUguale || c == motorea.CompatibilitaEquivalente || c == motorea.CompatibilitaParziale) && ma != "" && mb != "" && ma != mb {
		return motorea.CompatibilitaDiscordante
	}
	return c
}

// marcatoreDiForma: il marcatore scritto di una lettura; "" se non c'è.
func marcatoreDiForma(l *motorea.LetturaForma) string {
	if l == nil || l.Marcatore == nil {
		return ""
	}
	return l.Marcatore.Valore
}

// discordanze: le discordanze di revisione del 2D (T-E1R-09, E1R §5.4), in un ordine fisso: documento_componente
// calcolata in Go, documento_componente della vista, proposta_registrata, proposta_confermata.
func discordanze(d Disegno2D, c ComponenteDaConfrontare, fonti []FonteDelDisegno, dec *ancoraggio.DecisioneIdentita, vista *DiscordanzaRevisione) []DiscordanzaRevisione {
	var out []DiscordanzaRevisione
	if d.Revisione != nil && c.Revisione != nil && !stessaRevisione(*d.Revisione, *c.Revisione) {
		out = append(out, DiscordanzaRevisione{Tra: TraDocumentoComponente, A: strings.TrimSpace(*d.Revisione), B: strings.TrimSpace(*c.Revisione),
			FonteA: d.FonteIdentita, FonteB: c.FonteRevisione, Effetto: EffettoIndicatore, CalcolataDa: CalcolataDaGo})
	}
	if vista != nil {
		out = append(out, *vista)
	}
	p := d.RevisioneProposta
	if p.Valore == nil {
		return out
	}
	proposta := strings.TrimSpace(*p.Valore)
	if d.Registrata != nil && !stessaRevisione(proposta, *d.Registrata) {
		out = append(out, DiscordanzaRevisione{Tra: TraPropostaRegistrata, A: proposta, B: strings.TrimSpace(*d.Registrata),
			FonteA: p.Fonte, FonteB: FonteEvidenzaDocumento, Effetto: EffettoIndicatore, CalcolataDa: CalcolataDaGo})
	}
	if dec != nil {
		confermata := ""
		if d.Confermata != nil {
			confermata = *d.Confermata
		}
		if !stessaRevisione(proposta, confermata) {
			effetto := EffettoIndicatore
			if f := fonteDi(fonti, p.Fonte); f != nil && f.Coppia.Valore != "" && ancoraggio.EvidenzaNuova(*dec, f.Coppia) {
				effetto = EffettoConflitto
			}
			out = append(out, DiscordanzaRevisione{Tra: TraPropostaConfermata, A: proposta, B: confermata,
				FonteA: p.Fonte, FonteB: FonteDiscordanzaConfermata, Effetto: effetto, CalcolataDa: CalcolataDaGo})
		}
	}
	return out
}

// conflittiIdentitaDocumento: i conflitti identita_documento di un 2D (E1R §5.3; T-E1R-08; R95 A), come pezzi: una
// decisione sul documento contro un'evidenza nuova (la sua coppia non era fra le EvidenzeViste: ancoraggio.EvidenzaNuova,
// con la coppia di ancoraggio.EvidenzaDa, T-B4-22) che la contraddice. Per ogni fonte, nell'ordine (cartiglio,
// step_entita, nome_file), al più un conflitto:
//   - codice, se il codice letto dalla fonte e quello deciso, letti con la grammatica, sono discordanti (un'altra base,
//     o due marcatori scritti e diversi); lo STEP dell'entità non dà un codice del 2D (T-E1R-06), e senza la lettura
//     del codice deciso la parte codice non si giudica;
//   - altrimenti revisione, se la fonte ha una revisione interpretabile diversa da quella decisa (o la decisione è senza
//     revisione: l'assenza è decisa, T-B4-21).
//
// documento.rev non è mai un conflitto (T-E1R-07, E1R §5.5), e una fonte non interpretabile non inventa niente. La
// decisione resta il valore corrente; il conflitto porta la parte in Motivo, l'asse smistamento e le evidenze dei due
// lati (T-E1-15). Uno per prodotto; senza prodotti, uno con il prodotto vuoto. In ordine canonico.
func conflittiIdentitaDocumento(d Disegno2D, fonti []FonteDelDisegno, dec *ancoraggio.DecisioneIdentita, letturaDecisa *motorea.LetturaForma, prodotti []string) []Conflitto {
	if dec == nil {
		return nil
	}
	if len(prodotti) == 0 {
		prodotti = []string{""}
	}
	il := dec.Il.UTC().Truncate(time.Millisecond)
	da := dec.Da
	decisione := testoDeciso(ancoraggio.CodiceDeciso{Codice: strings.TrimSpace(dec.Codice), Rev: copiaTesto(d.Confermata)})
	var out []Conflitto
	for _, nome := range ordineFonti[:3] {
		f := fonteDi(fonti, nome)
		if f == nil || f.Coppia.Valore == "" || !ancoraggio.EvidenzaNuova(*dec, f.Coppia) {
			continue
		}
		parte := ""
		switch {
		case nome != FonteEvidenzaStepEntita && compatibilitaCodici(f.Lettura, letturaDecisa) == motorea.CompatibilitaDiscordante:
			parte = ancoraggio.ParteDiscordanzaCodice
		case revisioneUsabile(f.Revisione) != "" && (d.Confermata == nil || !stessaRevisione(revisioneUsabile(f.Revisione), *d.Confermata)):
			parte = ancoraggio.ParteDiscordanzaRevisione
		default:
			continue
		}
		var rif []string
		if f.Revisione.Entita != "" {
			rif = []string{f.Revisione.Entita}
		}
		for _, p := range prodotti {
			out = append(out, Conflitto{Tipo: ConflittoIdentitaDocumento, Asse: AsseSmistamento, Rif: RifDocumento(*d.DocumentoID), Prodotto: p,
				Decisione: decisione, OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: f.Coppia.Valore, Motivo: parte,
				EvidenzaDecisione: EvidenzaDecisione{Origine: ancoraggio.OrigineConfermato, RevProvenienza: ancoraggio.RevProvenienzaDecisioneTracciata,
					Da: copiaUUID(&da), Il: copiaTempo(&il)},
				EvidenzaProposta: EvidenzaProposta{AllegatoID: copiaUUID(f.AllegatoID), DocumentoID: copiaUUID(d.DocumentoID), UnitaID: f.UnitaID,
					Posizione: f.Posizione, Evidenza: f.Coppia, Riferimenti: rif}})
		}
	}
	ordinaConflitti(out)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Prodotto < out[j].Prodotto })
	return out
}
