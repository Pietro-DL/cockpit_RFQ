package grammatica

// Gli enum chiusi delle grammatiche v1 (parte 1 §5; R16, R17). Un valore fuori elenco è un errore di
// contratto (contratto.enum_ignoto), mai un ripiego: nessun adattatore legacy (R6, C-12). I campi dei DTO
// restano stringhe, perché il formato JSON delle grammatiche non dipenda dai tipi Go; queste costanti e gli
// elenchi sotto sono la sola fonte dei valori ammessi.

// Ruoli e categorie delle famiglie (v3 §2; R17).
const (
	RuoloProdotto   Ruolo = "prodotto"
	RuoloComponente Ruolo = "componente"

	CategoriaMinuteria Categoria = "minuteria"
)

// Tipi delle parti di una forma (parte 1 §5.1; marcatore, ripetizione_base e token da R16).
const (
	TipoParteEtichetta       = "etichetta"
	TipoParteAffisso         = "affisso"
	TipoParteBase            = "base"
	TipoParteRevisione       = "revisione"
	TipoParteDecorazione     = "decorazione"
	TipoParteSeparatore      = "separatore"
	TipoParteMarcatore       = "marcatore"
	TipoParteRipetizioneBase = "ripetizione_base"
	TipoParteToken           = "token"
)

// Tipi e sottotipi delle decorazioni. I primi cinque sono della parte 1 §5.2 e in A1 sono riservati
// (capacita.go); gli altri vengono dal v3 e dagli attesi (R16).
const (
	TipoDecorazioneFoglio       = "foglio"
	TipoDecorazioneFormato      = "formato"
	TipoDecorazioneSiglaInterna = "sigla_interna"
	TipoDecorazioneCopia        = "copia"
	TipoDecorazioneAnnotazione  = "annotazione"

	TipoDecorazioneInvolucro         = "involucro"
	TipoDecorazioneLivelloNomeFile   = "livello_nome_file"
	TipoDecorazioneStatoPDM          = "stato_pdm"
	TipoDecorazioneSuffissoDocumento = "suffisso_documento"
	TipoDecorazioneDescrizione       = "descrizione"

	// Sottotipi: solo per involucro (D2, D8).
	SottotipoRiferimentoPacchetto = "riferimento_pacchetto"
	SottotipoTecnico              = "tecnico"
)

// Posizione di un affisso, sorgente di una revisione, stato di una regola e del profilo.
const (
	PosizionePrefisso = "prefisso"
	PosizioneSuffisso = "suffisso"

	SorgenteInline        = "inline"
	SorgenteCampoSeparato = "campo_separato"

	StatoAttiva    = "attiva"
	StatoRiservata = "riservata"

	ProfiloCompleto = "completo"
	ProfiloParziale = "parziale"
)

// I due assi del qualificatore (parte 1 §5.3): l'assenza non vuol dire serie.
const (
	FasePrototipo    = "prototipo"
	FaseCampionatura = "campionatura"
	FasePreserie     = "preserie"
	FaseSerie        = "serie"

	DestinazioneRicambio = "ricambio"
)

// Le classi di confine (D-05). La _punto_cifra vale solo a destra.
const (
	ConfineAlnumASCII             ClasseConfine = "alnum_ascii"
	ConfineParolaASCII            ClasseConfine = "parola_ascii"
	ConfineAlnumASCIIOSpazio      ClasseConfine = "alnum_ascii_o_spazio"
	ConfineParolaASCIIOPuntoCifra ClasseConfine = "parola_ascii_o_punto_cifra"
)

// Maiuscole e normalizzazione della base; significato dei segmenti di revisione (D4); origine degli esempi
// (parte 1 §5.4); ambito dei qualificatori (parte 1 §5.3).
const (
	MaiuscoleEsatte            = "esatte"
	MaiuscoleIndifferentiASCII = "indifferenti_ascii"

	NormalizzaNessuna   = "nessuna"
	NormalizzaMaiuscolo = "maiuscolo"

	SignificatoForte   = "forte"
	SignificatoDebole  = "debole"
	SignificatoNessuno = "nessuno"

	OrigineSintetico = "sintetico"
	OrigineCasoReale = "caso_reale"

	AmbitoRiga            = "riga"
	AmbitoMessaggioLogico = "messaggio_logico"
	AmbitoCella           = "cella"
	AmbitoSezione         = "sezione"
)

// Gli elenchi chiusi, nell'ordine in cui il contratto li elenca. Servono a Valida e ai messaggi.
var (
	ruoli     = []string{string(RuoloProdotto), string(RuoloComponente)}
	categorie = []string{string(CategoriaMinuteria)}
	tipiParte = []string{TipoParteEtichetta, TipoParteAffisso, TipoParteBase, TipoParteRevisione, TipoParteDecorazione,
		TipoParteSeparatore, TipoParteMarcatore, TipoParteRipetizioneBase, TipoParteToken}
	tipiParteInterna = []string{TipoParteSeparatore, TipoParteToken, TipoParteRipetizioneBase} // dentro un suffisso_documento, un livello solo (D-03)
	tipiDecorazione  = []string{TipoDecorazioneFoglio, TipoDecorazioneFormato, TipoDecorazioneSiglaInterna, TipoDecorazioneCopia,
		TipoDecorazioneAnnotazione, TipoDecorazioneInvolucro, TipoDecorazioneLivelloNomeFile, TipoDecorazioneStatoPDM,
		TipoDecorazioneSuffissoDocumento, TipoDecorazioneDescrizione}
	sottotipiInvolucro = []string{SottotipoRiferimentoPacchetto, SottotipoTecnico}
	posizioni          = []string{PosizionePrefisso, PosizioneSuffisso}
	sorgenti           = []string{SorgenteInline, SorgenteCampoSeparato}
	stati              = []string{StatoAttiva, StatoRiservata}
	statiProfilo       = []string{ProfiloCompleto, ProfiloParziale}
	fasi               = []string{FasePrototipo, FaseCampionatura, FasePreserie, FaseSerie}
	destinazioni       = []string{DestinazioneRicambio}
	classiConfine      = []string{string(ConfineAlnumASCII), string(ConfineParolaASCII), string(ConfineAlnumASCIIOSpazio),
		string(ConfineParolaASCIIOPuntoCifra)}
	maiuscole       = []string{MaiuscoleEsatte, MaiuscoleIndifferentiASCII}
	normalizzazioni = []string{NormalizzaNessuna, NormalizzaMaiuscolo}
	significati     = []string{SignificatoForte, SignificatoDebole, SignificatoNessuno}
	origini         = []string{OrigineSintetico, OrigineCasoReale}
	ambiti          = []string{AmbitoRiga, AmbitoMessaggioLogico, AmbitoCella, AmbitoSezione}
)

// classiSoloADestra: le classi che hanno senso solo come confine destro (regola 12 della validazione).
var classiSoloADestra = []string{string(ConfineParolaASCIIOPuntoCifra)}

func in(v string, elenco []string) bool {
	for _, x := range elenco {
		if x == v {
			return true
		}
	}
	return false
}
