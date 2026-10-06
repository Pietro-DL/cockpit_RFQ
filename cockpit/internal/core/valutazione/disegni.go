package valutazione

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// I disegni 2D di un componente (B5, fase 2; R82, R83, R84; contratto §1.6, §2.3, §2.5, §2.6; T-B0-22, T-B0-26,
// T-B0-30, T-B0-31, T-E1-08, T-E1-21). Il requisito documentale è disegno_2d; la validità viene dal contenuto, il
// formato è una proprietà (contratto §0 n.5). Validità, anteprima e cartiglio sono tre cose (T-E1-21). Più 2D dello
// stesso componente restano tutti: niente si scarta, niente si fonde; il primario si calcola, le relazioni fra due 2D si
// propongono e restano da confermare (R84, T-E1-08). Le regole sono funzioni pure su ingressi astratti
// (ValiditaDisegno su ContenutoDisegno, Primario su []Disegno2D, ValutaDisegno su DisegnoDaValutare); il legacy entra
// solo dall'adattatore (disegni_thread.go: T-B0-22).

// ---- i formati (T-B0-30) ----

// Formato2D: il formato di un 2D, dall'estensione dichiarata (contratto §2.3; R82, T-B0-30). "" vuol dire «formato non
// configurato».
type Formato2D string

const (
	Formato2DPDF  Formato2D = "pdf"
	Formato2DTIFF Formato2D = "tiff"
	Formato2DPNG  Formato2D = "png"
)

// VersioneFormati2D: la versione dell'elenco chiuso dei formati (T-B0-30). Entra in Esito.Impronta con B6; si cambia
// solo con un commit che lo dichiara, insieme all'elenco.
const VersioneFormati2D = 1

// FormatoDisegno: un formato configurato con le sue estensioni, senza il punto e senza maiuscole.
type FormatoDisegno struct {
	Formato    Formato2D `json:"formato"`
	Estensioni []string  `json:"estensioni"`
}

// FormatiDisegno2D: l'elenco chiuso dei formati dei 2D (T-B0-30, VersioneFormati2D = 1), nell'ordine del criterio 3
// del primario: PDF (pdf), TIFF (tif, tiff), PNG (png). DWG, DFT e JPG non sono configurati e non soddisfano il
// fabbisogno 2D (formato_non_configurato, LD-02). Ogni chiamata dà una copia nuova: l'elenco non si cambia da fuori.
func FormatiDisegno2D() []FormatoDisegno {
	return []FormatoDisegno{
		{Formato: Formato2DPDF, Estensioni: []string{"pdf"}},
		{Formato: Formato2DTIFF, Estensioni: []string{"tif", "tiff"}},
		{Formato: Formato2DPNG, Estensioni: []string{"png"}},
	}
}

// FormatoDi: il formato configurato di un'estensione dichiarata (senza il punto davanti, senza maiuscole, senza gli
// spazi ai bordi); "" se l'estensione non è nell'elenco chiuso (T-B0-30: il formato è l'estensione dichiarata).
func FormatoDi(estensione string) Formato2D {
	e := normaEstensione(estensione)
	for _, f := range FormatiDisegno2D() {
		for _, x := range f.Estensioni {
			if x == e {
				return f.Formato
			}
		}
	}
	return ""
}

// normaEstensione: l'estensione senza gli spazi ai bordi, senza il punto davanti, senza maiuscole.
func normaEstensione(e string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), "."))
}

// rangoFormato: il criterio 3 del primario (R84): PDF, poi TIFF, poi PNG; un formato non configurato dopo tutti.
func rangoFormato(f Formato2D) int {
	switch f {
	case Formato2DPDF:
		return 3
	case Formato2DTIFF:
		return 2
	case Formato2DPNG:
		return 1
	}
	return 0
}

// ---- la validità (R83, T-B0-22, T-B0-31) ----

// ValiditaDisegno2D: la validità di un 2D (contratto §2.3; R83).
type ValiditaDisegno2D string

const (
	ValiditaValido                ValiditaDisegno2D = "valido"
	ValiditaDaVerificare          ValiditaDisegno2D = "da_verificare"
	ValiditaFormatoNonConfigurato ValiditaDisegno2D = "formato_non_configurato"
)

// MotivoFabbisogno: il motivo di una voce di un fabbisogno (contratto §2.3; era MotivoPDF; T-B0-31). I primi cinque
// sono anche i motivi della validità di un 2D (Disegno2D.MotivoValidita); gli altri li usa la completezza documentale
// (VoceFabbisogno, B5 fase 3).
type MotivoFabbisogno string

const (
	MotivoFabbisognoContenutoNonAnalizzato    MotivoFabbisogno = "contenuto_non_analizzato"
	MotivoFabbisognoContenutoNonAperto        MotivoFabbisogno = "contenuto_non_aperto"
	MotivoFabbisognoContenutoNonLetto         MotivoFabbisogno = "contenuto_non_letto"
	MotivoFabbisognoContenutoNonVerificabile  MotivoFabbisogno = "contenuto_non_verificabile"
	MotivoFabbisognoFormatoNonConfigurato     MotivoFabbisogno = "formato_non_configurato"
	MotivoFabbisognoAssociazioneNonConfermata MotivoFabbisogno = "associazione_non_confermata"
	MotivoFabbisognoNessunDocumento           MotivoFabbisogno = "nessun_documento"
	MotivoFabbisognoDerogatoNonSostituisce2D  MotivoFabbisogno = "derogato_non_sostituisce_2d"
	MotivoFabbisognoSulPortale                MotivoFabbisogno = "sul_portale"
)

// ContenutoDisegno: il contenuto di un 2D come lo vede la regola della validità, il suo ingresso astratto (contratto
// §2.3; T-B0-22). In A1c lo producono solo i fatti dei PDF (contenutoDelDisegno); per le immagini nessun fatto di
// decodifica (LD-01), e la regola si prova con contenuti sintetici (PO-08, PO-09).
//   - Formato: il formato configurato; "" per un formato non configurato.
//   - Decodificato: il contenuto si apre (un PDF con testo_pdf, anche una scansione senza testo; un'immagine
//     decodificata).
//   - Testo: c'è testo nel file (un PDF con testo nativo).
//   - Motivo: perché non è decodificato (uno dei motivi della validità di T-B0-31); "" quando lo è.
type ContenutoDisegno struct {
	Formato      Formato2D        `json:"formato,omitempty"`
	Decodificato bool             `json:"decodificato"`
	Testo        bool             `json:"testo"`
	Motivo       MotivoFabbisogno `json:"motivo,omitempty"`
}

// ValiditaDisegno: la regola della validità di un 2D (R83; T-B0-30, T-B0-31; contratto §1.6), il primo che vale:
//   - un formato fuori dall'elenco chiuso: formato_non_configurato, con lo stesso motivo;
//   - un contenuto decodificato: valido, qualunque sia il testo (una scansione senza OCR è valida: PO-10);
//   - altrimenti da_verificare, con il motivo del contenuto (contenuto_non_analizzato, contenuto_non_aperto,
//     contenuto_non_letto, contenuto_non_verificabile); un motivo fuori da questi quattro, o nessuno, vale
//     contenuto_non_verificabile, il più prudente: il contenuto non si dice né aperto né letto.
//
// È pura: guarda il contenuto, mai l'identità (un cartiglio letto male non annulla niente: R62 g A).
func ValiditaDisegno(c ContenutoDisegno) (ValiditaDisegno2D, MotivoFabbisogno) {
	if rangoFormato(c.Formato) == 0 {
		return ValiditaFormatoNonConfigurato, MotivoFabbisognoFormatoNonConfigurato
	}
	if c.Decodificato {
		return ValiditaValido, ""
	}
	switch c.Motivo {
	case MotivoFabbisognoContenutoNonAnalizzato, MotivoFabbisognoContenutoNonAperto, MotivoFabbisognoContenutoNonLetto,
		MotivoFabbisognoContenutoNonVerificabile:
		return ValiditaDaVerificare, c.Motivo
	}
	return ValiditaDaVerificare, MotivoFabbisognoContenutoNonVerificabile
}

// ---- il cartiglio e l'anteprima (T-E1-21, LD-03, LD-04) ----

// I valori di Disegno2D.Cartiglio (contratto §2.5; T-E1-21):
//   - letto: il PDF ha almeno un campo del cartiglio fra le unità, dal testo nativo o dall'OCR (dubbio T-B5-37);
//   - non_leggibile: un PDF senza cartiglio leggibile (una scansione senza OCR, un PDF che non si apre o non analizzato,
//     un testo senza campi del cartiglio, il testo del worker di prima, i cui campi sono unità testo_pdf);
//   - formato_senza_lettura: un formato di cui oggi il cartiglio non si legge (TIFF, PNG, i formati non configurati:
//     LD-04, nessun localizzatore raster).
const (
	CartiglioLetto               = "letto"
	CartiglioNonLeggibile        = "non_leggibile"
	CartiglioFormatoSenzaLettura = "formato_senza_lettura"
)

// RiferimentoAnteprima: il riferimento all'originale per l'anteprima (contratto §2.3; R83, R91). L'originale resta
// sempre quello: Sha256 e, se ci sono, il documento e l'allegato. Disponibile: oggi solo per il PDF che si apre (LD-03:
// il web mostra solo i PDF; la conversione delle immagini viene dopo A1c; un PDF con errore_pdf no: T-B5-38).
type RiferimentoAnteprima struct {
	Sha256      string     `json:"sha256"`
	Formato     Formato2D  `json:"formato,omitempty"`
	DocumentoID *uuid.UUID `json:"documento_id,omitempty"`
	AllegatoID  *uuid.UUID `json:"allegato_id,omitempty"`
	Disponibile bool       `json:"disponibile"`
}

// ---- il 2D ----

// I valori di Disegno2D.FonteIdentita (contratto §2.3, §2.6; T-E1R-06): da dove viene la revisione della compatibilità
// (e il codice del 2D): la decisione sul documento, poi le evidenze proprie del 2D (il cartiglio, il nome del file),
// poi documento.rev come ultima risorsa. Mai lo STEP dell'entità.
const (
	FonteIdentitaConfermata = "confermata"
	FonteIdentitaCartiglio  = "cartiglio"
	FonteIdentitaNomeFile   = "nome_file"
	FonteIdentitaDocumento  = "documento"
)

// I valori di Disegno2D.ConfrontatoCon (contratto §2.3; T-B0-34): con quale revisione del componente il 2D è stato
// confrontato nel criterio 2: decisa (una decisione sul componente, o componente.rev registrata), proposta (l'identità
// del nodo del componente proposta dallo STEP), nessuna.
const (
	ConfrontatoConDeciso   = "deciso"
	ConfrontatoConProposto = "proposto"
	ConfrontatoConNessuno  = "nessuno"
)

// Disegno2D: un 2D di un componente, con il formato come proprietà (contratto §2.3, §2.5, §2.6; R83, R84, R104).
//   - DocumentoID, AllegatoID, Sha256, NomeFile, Estensione: il file. Un documento confermato porta il suo ID e, se uno
//     dei suoi allegati è fra i file del thread, l'allegato; un candidato porta solo l'allegato. Estensione è quella
//     dichiarata, senza il punto e senza maiuscole.
//   - Formato, Validita, MotivoValidita, Testo: dal contenuto (ValiditaDisegno; T-B0-30, T-B0-31).
//   - Cartiglio: letto, non_leggibile o formato_senza_lettura (T-E1-21): un 2D valido senza cartiglio resta valido.
//   - Codice, Revisione, FonteIdentita: l'identità del 2D per la compatibilità (T-E1R-06): la decisione sul documento,
//     poi il cartiglio, poi il nome del file, poi documento.rev (e documento.codice) come ultima risorsa; mai lo STEP.
//     FonteIdentita dice da dove viene la revisione; il codice è il testo letto nella prima fonte che lo dà.
//   - Provenienza: l'origine dell'associazione al componente: confermato (un documento confermato sul componente),
//     manuale («assegna»), proposto (un candidato del motore A sul nodo del componente).
//   - Corrente: un documento confermato e non sostituito (sostituito_da nullo). ConfermatoIl: quando è stato
//     confermato; nil per un 2D che non è un documento.
//   - CompatibilitaCodice, CompatibilitaRevisione, ConfrontatoCon: il criterio 2 del primario, contro il componente
//     (motorea.Compatibilita; non_determinabile quando manca un lato: T-B0-34).
//   - Anteprima: il riferimento all'originale (LD-03).
//   - Confermata, CodiceConfermato: la revisione e il codice decisi sul documento (DecisioneIdentita con l'oggetto
//     documento, T-E1R-08); nil senza decisione. In A1c nessun adattatore la produce (LD-27).
//   - EvidenzeRevisione: le evidenze lette, una per fonte (cartiglio, step_entita, nome_file, documento).
//     RevisioneProposta: la proposta (E1R §5.2). Registrata, RevProvenienza: documento.rev con la provenienza
//     (T-E1R-07). StatoRevisione: il livello più alto che c'è. Discordanze: sempre calcolate (T-E1R-09).
type Disegno2D struct {
	DocumentoID            *uuid.UUID             `json:"documento_id,omitempty"`
	AllegatoID             *uuid.UUID             `json:"allegato_id,omitempty"`
	Sha256                 string                 `json:"sha256"`
	NomeFile               string                 `json:"nome_file"`
	Formato                Formato2D              `json:"formato,omitempty"`
	Estensione             string                 `json:"estensione"`
	Validita               ValiditaDisegno2D      `json:"validita"`
	MotivoValidita         MotivoFabbisogno       `json:"motivo_validita,omitempty"`
	Testo                  bool                   `json:"testo"`
	Cartiglio              string                 `json:"cartiglio"`
	Codice                 string                 `json:"codice,omitempty"`
	Revisione              *string                `json:"revisione,omitempty"`
	FonteIdentita          string                 `json:"fonte_identita,omitempty"`
	Provenienza            ancoraggio.OrigineDato `json:"provenienza"`
	Corrente               bool                   `json:"corrente"`
	ConfermatoIl           *time.Time             `json:"confermato_il,omitempty"`
	CompatibilitaCodice    motorea.Compatibilita  `json:"compatibilita_codice"`
	CompatibilitaRevisione motorea.Compatibilita  `json:"compatibilita_revisione"`
	ConfrontatoCon         string                 `json:"confrontato_con"`
	Anteprima              RiferimentoAnteprima   `json:"anteprima"`
	Confermata             *string                `json:"confermata,omitempty"`
	CodiceConfermato       *string                `json:"codice_confermato,omitempty"`
	EvidenzeRevisione      []EvidenzaRevisione    `json:"evidenze_revisione,omitempty"`
	RevisioneProposta      RevisioneProposta      `json:"revisione_proposta"`
	Registrata             *string                `json:"registrata,omitempty"`
	RevProvenienza         string                 `json:"rev_provenienza,omitempty"`
	StatoRevisione         string                 `json:"stato_revisione"`
	Discordanze            []DiscordanzaRevisione `json:"discordanze,omitempty"`
}

// ---- il gruppo, il primario, le relazioni (R84, T-B0-26, T-E1-08, T-E1R-06) ----

// I valori di GruppoDisegni2D.RegolaPrimario (contratto §2.3): il criterio che separa il primario dal primo degli
// alternativi. "" con un 2D solo: nessuna scelta.
const (
	RegolaPrimarioCorrente      = "corrente"
	RegolaPrimarioCompatibilita = "compatibilita"
	RegolaPrimarioFormato       = "formato"
	RegolaPrimarioRecente       = "recente"
	RegolaPrimarioSha256        = "sha256"
)

// GruppoDisegni2D: i 2D di un componente (contratto §2.3; R84): niente si scarta, niente si fonde. Primario: il primo
// nell'ordine di Primario; nil per un gruppo vuoto. Alternativi: gli altri, nello stesso ordine. RegolaPrimario: il
// criterio che ha deciso. RelazioniDaConfermare: una per coppia di 2D, sempre da confermare (T-E1-08).
type GruppoDisegni2D struct {
	Primario              *Disegno2D         `json:"primario,omitempty"`
	Alternativi           []Disegno2D        `json:"alternativi,omitempty"`
	RegolaPrimario        string             `json:"regola_primario,omitempty"`
	RelazioniDaConfermare []RelazioneDisegni `json:"relazioni_da_confermare,omitempty"`
}

// I valori di RelazioneDisegni.Tipo e .Letture (contratto §2.5; T-E1-08) e di RelazioneDisegni.Stato. riemissione è
// una lettura possibile (T-E1-08, PO-22), non un tipo.
const (
	RelazioneRappresentazioneAlternativa = "rappresentazione_alternativa"
	RelazioneRevisioneDiversa            = "revisione_diversa"
	RelazioneElaboratoAggiuntivo         = "elaborato_aggiuntivo"
	RelazioneNonDeterminata              = "non_determinata"
	LetturaRiemissione                   = "riemissione"
	StatoRelazioneDaConfermare           = "da_confermare"
)

// RelazioneDisegni: la relazione proposta fra due 2D dello stesso componente (contratto §2.3, §2.5; T-E1-08). A, B: gli
// sha256 dei due, A < B. Tipo: proposto solo quando le evidenze lo sostengono, altrimenti non_determinata con le
// Letture possibili. Stato: sempre da_confermare. Mai una sostituzione, una revisione tecnica, un cambio di revisione.
type RelazioneDisegni struct {
	A       string   `json:"a"`
	B       string   `json:"b"`
	Tipo    string   `json:"tipo"`
	Letture []string `json:"letture,omitempty"`
	Stato   string   `json:"stato"`
}

// Primario: il gruppo dei 2D di un componente con il primario (R84; contratto §1.6 e §2.3, «Primario([]Disegno2D)»).
// L'ordine, il primo criterio che decide:
//  1. corrente e confermato (Provenienza confermato e Corrente);
//  2. compatibile per codice e revisione con il componente (CompatibilitaCodice, poi CompatibilitaRevisione: uguale o
//     equivalente, poi compatibile_parziale, poi non_determinabile, poi discordante). La revisione del 2D è quella
//     della compatibilità (T-E1R-06: confermata, cartiglio, nome del file, documento.rev), mai la proposta dallo STEP:
//     un PDF vecchio senza revisione propria non vince su un TIFF della revisione giusta (E1R §5.6, PO-38);
//  3. il formato: prima la validità (valido, poi da_verificare, poi formato_non_configurato), poi PDF, TIFF, PNG (un
//     formato non configurato dopo), con la regola formato: il PDF è il preferito solo fra documenti compatibili,
//     validi e correnti (la precisazione dell'utente su R84 nell'emendamento E1; dubbio T-B5-39 corretto con la
//     revisione della fase 2). La validità non viene prima del criterio 1: un DWG confermato e corrente resta prima di
//     un PDF solo proposto, come vuole l'ordine di R84;
//  4. il confermato più di recente, poi lo sha256 (T-B0-26), poi il documento e l'allegato (un ordine fisso anche fra
//     due voci senza sha256).
//
// Le relazioni: una per coppia di 2D con lo sha256 (T-E1-08), con relazioneFra. È pura; i 2D di chi chiama non
// cambiano. Chi chiama passa i 2D di un componente, uno per contenuto.
func Primario(disegni []Disegno2D) GruppoDisegni2D {
	if len(disegni) == 0 {
		return GruppoDisegni2D{}
	}
	ordinati := make([]Disegno2D, len(disegni))
	for i := range disegni {
		ordinati[i] = copiaDisegno(disegni[i])
	}
	sort.SliceStable(ordinati, func(i, j int) bool { return confrontaPrimario(ordinati[i], ordinati[j]) < 0 })
	g := GruppoDisegni2D{Primario: &ordinati[0]}
	if len(ordinati) > 1 {
		g.Alternativi = ordinati[1:]
		g.RegolaPrimario = criterioPrimario(ordinati[0], ordinati[1])
	}
	for i := range ordinati {
		for j := i + 1; j < len(ordinati); j++ {
			if r, ok := relazioneFra(ordinati[i], ordinati[j]); ok {
				g.RelazioniDaConfermare = append(g.RelazioniDaConfermare, r)
			}
		}
	}
	sort.SliceStable(g.RelazioniDaConfermare, func(i, j int) bool {
		a, b := g.RelazioniDaConfermare[i], g.RelazioniDaConfermare[j]
		if a.A != b.A {
			return a.A < b.A
		}
		return a.B < b.B
	})
	return g
}

// criteriPrimario: i criteri del primario, nell'ordine, ognuno con il suo nome: il valore più grande vince.
var criteriPrimario = []struct {
	regola string
	valore func(Disegno2D) int
}{
	{RegolaPrimarioCorrente, func(d Disegno2D) int {
		if d.Provenienza == ancoraggio.OrigineConfermato && d.Corrente {
			return 1
		}
		return 0
	}},
	{RegolaPrimarioCompatibilita, func(d Disegno2D) int { return rangoCompatibilita(d.CompatibilitaCodice) }},
	{RegolaPrimarioCompatibilita, func(d Disegno2D) int { return rangoCompatibilita(d.CompatibilitaRevisione) }},
	{RegolaPrimarioFormato, func(d Disegno2D) int { return rangoValidita(d.Validita) }},
	{RegolaPrimarioFormato, func(d Disegno2D) int { return rangoFormato(d.Formato) }},
}

// rangoValidita: la validità nel criterio del formato (R84 con la precisazione E1): valido, poi da_verificare, poi
// formato_non_configurato (e una validità che non c'è).
func rangoValidita(v ValiditaDisegno2D) int {
	switch v {
	case ValiditaValido:
		return 2
	case ValiditaDaVerificare:
		return 1
	}
	return 0
}

// confrontaPrimario: <0 se a viene prima di b nel gruppo, >0 se dopo, 0 se sono la stessa voce.
func confrontaPrimario(a, b Disegno2D) int {
	for _, c := range criteriPrimario {
		if va, vb := c.valore(a), c.valore(b); va != vb {
			return vb - va
		}
	}
	if r := confrontaRecente(a.ConfermatoIl, b.ConfermatoIl); r != 0 {
		return r
	}
	for _, k := range [][2]string{{a.Sha256, b.Sha256}, {testoUUID(a.DocumentoID), testoUUID(b.DocumentoID)}, {testoUUID(a.AllegatoID), testoUUID(b.AllegatoID)}} {
		if k[0] != k[1] {
			return strings.Compare(k[0], k[1])
		}
	}
	return 0
}

// criterioPrimario: il criterio che mette il primo prima del secondo (RegolaPrimario).
func criterioPrimario(a, b Disegno2D) string {
	for _, c := range criteriPrimario {
		if c.valore(a) != c.valore(b) {
			return c.regola
		}
	}
	if confrontaRecente(a.ConfermatoIl, b.ConfermatoIl) != 0 {
		return RegolaPrimarioRecente
	}
	return RegolaPrimarioSha256
}

// confrontaRecente: il confermato più di recente prima; un 2D senza la data della conferma dopo.
func confrontaRecente(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	case a.After(*b):
		return -1
	case b.After(*a):
		return 1
	}
	return 0
}

// rangoCompatibilita: il criterio 2 (R84): uguale o equivalente, poi compatibile_parziale, poi non_determinabile (né
// sì né no: T-B0-34), poi discordante.
func rangoCompatibilita(c motorea.Compatibilita) int {
	switch c {
	case motorea.CompatibilitaUguale, motorea.CompatibilitaEquivalente:
		return 3
	case motorea.CompatibilitaParziale:
		return 2
	case motorea.CompatibilitaDiscordante:
		return 0
	}
	return 1
}

// relazioneFra: la relazione proposta fra due 2D dello stesso componente (T-E1-08), solo fra due 2D con lo sha256 e
// diversi. Le revisioni sono quelle della compatibilità (Disegno2D.Revisione, T-E1R-06); la stessa identità vuol dire
// che tutti e due hanno il codice uguale (o equivalente) a quello del componente (CompatibilitaCodice):
//   - due revisioni diverse, e nessuno dei due con il codice discordante: revisione_diversa (con un codice discordante
//     il 2D è di un'altra identità, e le evidenze non sostengono il tipo: si va avanti);
//   - la stessa identità e la stessa revisione, un formato diverso: rappresentazione_alternativa;
//   - la stessa identità, la stessa revisione e lo stesso formato, un contenuto diverso: non_determinata, con le letture
//     elaborato_aggiuntivo, revisione_diversa, riemissione (PO-22);
//   - altrimenti (una revisione o l'identità che non si sa): non_determinata, con le letture possibili: con lo stesso
//     formato come sopra; con un formato diverso rappresentazione_alternativa, revisione_diversa, elaborato_aggiuntivo.
//
// elaborato_aggiuntivo è sempre una lettura da confermare, mai una deduzione.
func relazioneFra(a, b Disegno2D) (RelazioneDisegni, bool) {
	if a.Sha256 == "" || b.Sha256 == "" || a.Sha256 == b.Sha256 {
		return RelazioneDisegni{}, false
	}
	r := RelazioneDisegni{A: a.Sha256, B: b.Sha256, Stato: StatoRelazioneDaConfermare}
	if r.B < r.A {
		r.A, r.B = r.B, r.A
	}
	stessoFormato := a.Formato == b.Formato
	revisioni := a.Revisione != nil && b.Revisione != nil
	stessaIdentita := rangoCompatibilita(a.CompatibilitaCodice) == 3 && rangoCompatibilita(b.CompatibilitaCodice) == 3
	discordante := a.CompatibilitaCodice == motorea.CompatibilitaDiscordante || b.CompatibilitaCodice == motorea.CompatibilitaDiscordante
	switch {
	case revisioni && !discordante && !stessaRevisione(*a.Revisione, *b.Revisione):
		r.Tipo = RelazioneRevisioneDiversa
	case revisioni && stessaIdentita && !stessoFormato:
		r.Tipo = RelazioneRappresentazioneAlternativa
	case stessoFormato:
		r.Tipo, r.Letture = RelazioneNonDeterminata, []string{RelazioneElaboratoAggiuntivo, RelazioneRevisioneDiversa, LetturaRiemissione}
	default:
		r.Tipo, r.Letture = RelazioneNonDeterminata, []string{RelazioneRappresentazioneAlternativa, RelazioneRevisioneDiversa, RelazioneElaboratoAggiuntivo}
	}
	return r, true
}

// stessaRevisione: due revisioni uguali, esatte dopo gli spazi ai bordi, come le confronta ancoraggio
// (riconciliazione, T-E1R-09) e come le tiene la grammatica (la normalizzata è il testo letto, senza cambiare le
// maiuscole: «a» e «A» sono due revisioni). Vale per le letture e per i valori registrati (dubbio T-B5-34 corretto con
// la revisione della fase 2). upper(btrim()) resta solo della vista (v_fascicolo.rev_diversa), che si riporta com'è
// (revDiversaDellaVista). Un'equivalenza dichiarata dalla grammatica non si vede qui.
func stessaRevisione(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

// ---- le copie: l'uscita non condivide memoria con l'ingresso ----

func copiaDisegno(d Disegno2D) Disegno2D {
	d.DocumentoID, d.AllegatoID = copiaUUID(d.DocumentoID), copiaUUID(d.AllegatoID)
	d.Revisione, d.Confermata, d.CodiceConfermato, d.Registrata = copiaTesto(d.Revisione), copiaTesto(d.Confermata), copiaTesto(d.CodiceConfermato), copiaTesto(d.Registrata)
	d.ConfermatoIl = copiaTempo(d.ConfermatoIl)
	d.Anteprima.DocumentoID, d.Anteprima.AllegatoID = copiaUUID(d.Anteprima.DocumentoID), copiaUUID(d.Anteprima.AllegatoID)
	if d.EvidenzeRevisione != nil {
		ev := make([]EvidenzaRevisione, len(d.EvidenzeRevisione))
		for i, e := range d.EvidenzeRevisione {
			e.Valore = copiaTesto(e.Valore)
			ev[i] = e
		}
		d.EvidenzeRevisione = ev
	}
	d.RevisioneProposta.Valore = copiaTesto(d.RevisioneProposta.Valore)
	d.Discordanze = append([]DiscordanzaRevisione(nil), d.Discordanze...)
	return d
}
