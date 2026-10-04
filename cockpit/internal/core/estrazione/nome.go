package estrazione

import (
	"fmt"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
)

// Il nome del file e la voce d'archivio (mappatura-nome-1; 5.4.5; R19 d).
const (
	campoNomeFile = "allegato.nome_file"    // il testo originale e il campo di PosNomeFile del nome
	campoPercorso = "allegato.path_interno" // il testo originale e il campo di PosNomeFile della voce

	idUnitaNome     = "u:nome"
	idUnitaPercorso = "u:percorso"

	// maxRuneNome: il nome di un allegato diretto si tronca a 300 rune all'acquisizione (ingest.go:895). Un
	// nome che ci arriva può essere stato tagliato: si dice, non si ricostruisce.
	maxRuneNome = 300

	motivoNomeDalDB = "il nome com'è nel DB, già trasformato dall'acquisizione (Provenienza.Ignoti)"
)

// estensioniNonEstratte: gli archivi che l'acquisizione non apre (platform/storage/archivio/zip.go:1-2): le loro
// voci non esistono come allegati, e A lo dice (REG §5.10, «Conseguenze»).
var estensioniNonEstratte = map[string]bool{"7z": true, "rar": true}

// dividiNome è la regola dello stem dichiarata dal motore A (C-33), una sola al posto delle quattro di oggi
// (proposta.go:38-39, codici.go:503-507, archivi.go:97, ingest.go:884-889). È quella di os.path.splitext del
// worker (outlook_com.py:967), con una differenza dichiarata:
//   - si guarda l'ultimo pezzo del percorso, dopo l'ultima «/» (path_interno usa «/» dopo zip.go:67);
//   - l'estensione è il testo dopo l'ultimo «.» di quel pezzo, se il punto non sta fra i punti iniziali del
//     pezzo («.nascosto» non ha estensione) e se dopo c'è almeno un carattere («nome.» non ha estensione:
//     splitext darebbe «.», qui no);
//   - lo stem è il resto, dall'inizio del testo fino al punto escluso: per un percorso comprende le cartelle,
//     come in splitext. Senza estensione lo stem è tutto il testo.
//
// Gli intervalli sono in byte sul testo dato, e cadono su confini di runa perché «/» e «.» sono ASCII. Un testo
// vuoto non ha né stem né estensione.
func dividiNome(p string) (stem, estensione *evidenze.Intervallo) {
	if p == "" {
		return nil, nil
	}
	tutto := intero(p)
	inizioPezzo := strings.LastIndexByte(p, '/') + 1
	punto := strings.LastIndexByte(p, '.')
	if punto < inizioPezzo || punto == len(p)-1 {
		return &tutto, nil
	}
	i := inizioPezzo
	for i < punto && p[i] == '.' {
		i++
	}
	if i == punto {
		return &tutto, nil // il punto è uno dei punti iniziali del pezzo
	}
	return &evidenze.Intervallo{Inizio: 0, Fine: punto}, &evidenze.Intervallo{Inizio: punto + 1, Fine: len(p)}
}

// estensioneCalcolata: l'estensione della regola, in minuscolo; "" se non c'è.
func estensioneCalcolata(p string) string {
	_, est := dividiNome(p)
	if est == nil {
		return ""
	}
	return strings.ToLower(p[est.Inizio:est.Fine])
}

// posNome: il localizzatore di un nome o di un percorso, con gli intervalli del testo intero, dello stem e
// dell'estensione, misurati sul testo originale che ha per ID il campo.
func posNome(campo, testo, percorso string) evidenze.Localizzatore {
	stem, est := dividiNome(testo)
	return evidenze.Localizzatore{Tipo: "nome_file", NomeFile: &evidenze.PosNomeFile{
		Campo:      campo,
		Percorso:   percorso,
		Intervallo: intero(testo),
		Stem:       stem,
		Estensione: est,
	}}
}

// nomeDelFile aggiunge al documento le unità del nome (5.4.5, R19 d) e le loro diagnostiche, e restituisce
// l'estensione calcolata, che decide il resto dell'adattatore:
//   - u:nome: il nome com'è nel DB (allegato.nome_file), selettore nome_file, localizzazione esatta;
//   - u:percorso, solo per una voce d'archivio con il percorso: il percorso intero (allegato.path_interno),
//     selettore voce_archivio. Il nome del contenitore non entra: lo legge il documento del contenitore;
//   - nome.estensione_discorde, nome.forse_troncato, archivio.non_estraibile.
func (c *documento) nomeDelFile(a fotorfq.Allegato) string {
	voce := a.ContenitoreID != nil
	percorso := ""
	if voce && a.PathInterno != nil {
		percorso = *a.PathInterno
	}

	c.testo(campoNomeFile, a.NomeFile, campoNomeFile)
	c.unita(evidenze.UnitaEvidenza{
		ID:             idUnitaNome,
		Selettore:      selettoreDi(evidenze.ContestoNomeFile),
		Testo:          a.NomeFile,
		CampoOriginale: evidenze.CampoOriginale{Parser: campoNomeFile, Mappatura: mappaturaNome},
		Posizione:      posNome(campoNomeFile, a.NomeFile, percorso),
		Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneEsatta, Motivo: motivoNomeDalDB, Metodo: metodoAdattatore},
	})
	if percorso != "" {
		c.testo(campoPercorso, percorso, campoPercorso)
		c.unita(evidenze.UnitaEvidenza{
			ID:             idUnitaPercorso,
			Selettore:      selettoreDi(evidenze.ContestoVoceArchivio),
			Testo:          percorso,
			CampoOriginale: evidenze.CampoOriginale{Parser: campoPercorso, Mappatura: mappaturaNome},
			Posizione:      posNome(campoPercorso, percorso, percorso),
			Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneEsatta, Motivo: motivoNomeDalDB, Metodo: metodoAdattatore},
		})
	}

	est := estensioneCalcolata(a.NomeFile)
	registrata := ""
	if a.Estensione != nil {
		// NULL nel DB è l'estensione vuota: l'acquisizione scrive NULL al posto di "" (txtN, ingest.go:640-645).
		registrata = *a.Estensione
	}
	if est != registrata {
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceNomeEstensioneDiscorde,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  campoNomeFile,
			Messaggio: fmt.Sprintf("l'estensione della regola dello stem è %q, allegato.estensione è %q", est, registrata),
			Rif:       []string{idUnitaNome},
		})
	}
	if !voce && runeDi(a.NomeFile) >= maxRuneNome {
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceNomeForseTroncato,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  campoNomeFile,
			Messaggio: fmt.Sprintf("il nome ha %d rune: l'acquisizione tronca a %d, e la fine può mancare", runeDi(a.NomeFile), maxRuneNome),
			Rif:       []string{idUnitaNome},
		})
	}
	if estensioniNonEstratte[est] {
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceArchivioNonEstraibile,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaCapacita,
			Percorso:  campoNomeFile,
			Messaggio: fmt.Sprintf("archivio .%s: l'acquisizione non ne estrae le voci, che quindi non sono allegati da leggere", est),
			Rif:       []string{idUnitaNome},
		})
	}
	return est
}

// selettoreDi: il selettore di un contesto senza campi (variante «nessuno»).
func selettoreDi(ctx evidenze.Contesto) evidenze.Selettore {
	return evidenze.Selettore{Contesto: ctx, Campo: evidenze.CampoFonte{Variante: evidenze.VarianteNessuno}}
}
