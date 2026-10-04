package estrazione

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I tipi della fonte e del riferimento ai fatti (Fonte.Tipo, RiferimentoFatti.Tipo): i valori chiusi della
// foglia, che la foglia non esporta. «testo» è la fonte sintetica di DaTesto: OrigineID nullo, riferimento
// «nessuno», un documento che non si salva mai (5.4.1).
const (
	fonteMessaggio = "messaggio"
	fonteAllegato  = "allegato"
	fonteTesto     = "testo"

	riferimentoAnalisiFile = "analisi_file"
	riferimentoMessaggio   = "messaggio"
	riferimentoNessuno     = "nessuno"
)

// Ciò che una fonte non può dire (Provenienza.Ignoti). Sono frasi fisse: entrano nel BundleID, quindi si
// cambiano solo con VersioneAdattatore. Dicono anche le trasformazioni dell'acquisizione, perché il nome e il
// percorso del documento sono quelli già trasformati, e l'originale non c'è più (5.4.5, «Mappatura nome file»).
const (
	ignotoAutoreAllegato  = "autore: il record dell'allegato non lo porta"
	ignotoNomeDiretto     = "nome come l'ha dato la posta: l'acquisizione lo tronca a 300 rune e toglie gli spazi ai bordi (ingest.go:895, :640-645)"
	ignotoNomeGrezzoVoce  = "nome grezzo della voce nell'archivio (byte, CP-437): il nome è path.Base del percorso pulito, senza trim né troncamento (zip.go:71)"
	ignotoPercorsoGrezzo  = "percorso come sta nell'archivio: l'acquisizione converte «\\» in «/» e applica path.Clean (zip.go:67)"
	ignotoPercorsoVoce    = "percorso della voce nell'archivio: path_interno assente"
	ignotoCanale          = "canale: non esportato"
	ignotoAutoreMessaggio = "autore: il mittente non c'è"
	ignotoCorpoGrezzo     = "corpo_testo come l'ha dato la posta: l'acquisizione toglie gli spazi ai bordi (ingest.go:812)"
	ignotoOggettoGrezzo   = "oggetto come l'ha dato la posta: l'acquisizione lo tronca a 500 rune e toglie gli spazi ai bordi (ingest.go:812)"
)

// idFonte: l'ID di una fonte contiene il tipo, «allegato:<uuid>» (par.3.3.2). Lo stesso sha256 non fonde due
// fonti: due acquisizioni restano due fonti, con due ID (HASH-CONFLITTO).
func idFonte(tipo string, id uuid.UUID) string { return tipo + ":" + id.String() }

// fonteDiAllegato: la fonte di un file, con la provenienza e il riferimento ai fatti.
//   - Una voce d'archivio ha il contenitore: Fonte.ContenitoreID è «allegato:<uuid dello zip>». Se il record del
//     contenitore è dato, deve essere proprio quello: un contenitore diverso è un errore, non un'altra voce.
//     Un allegato diretto ha per contenitore il suo messaggio, «messaggio:<uuid>».
//   - Un percorso interno senza contenitore è un record incoerente, e un errore: il percorso di una voce si
//     legge solo dentro il suo archivio.
//   - I fatti devono essere del contenuto dell'allegato (stesso sha256, se l'allegato lo porta) e, se Digest è
//     già calcolato, avere quell'impronta: dei fatti attaccati al file sbagliato sarebbero una relazione
//     inventata (5.0).
func fonteDiAllegato(a fotorfq.Allegato, f *fotorfq.Fatti, contenitore *fotorfq.Allegato) (evidenze.Fonte, error) {
	fonte := evidenze.Fonte{
		ID:        idFonte(fonteAllegato, a.ID),
		Tipo:      fonteAllegato,
		OrigineID: a.ID,
		Provenienza: evidenze.Provenienza{
			OrigineDichiarata: a.Origine,
			NomeRicevuto:      a.NomeFile,
			Data:              a.RicevutoIl,
			Bytes:             a.Bytes,
			Ignoti:            []string{ignotoAutoreAllegato},
		},
	}
	if a.ContentType != nil {
		fonte.Provenienza.ContentType = *a.ContentType
	}
	switch {
	case a.ContenitoreID != nil:
		if contenitore != nil && contenitore.ID != *a.ContenitoreID {
			return evidenze.Fonte{}, fmt.Errorf("estrazione: allegato %s: il contenitore dato (%s) non è quello del record (%s)", a.ID, contenitore.ID, *a.ContenitoreID)
		}
		fonte.ContenitoreID = idFonte(fonteAllegato, *a.ContenitoreID)
		fonte.Provenienza.Ignoti = append(fonte.Provenienza.Ignoti, ignotoNomeGrezzoVoce)
		if a.PathInterno != nil && *a.PathInterno != "" {
			fonte.Provenienza.PercorsoRicevuto = *a.PathInterno
			fonte.Provenienza.Ignoti = append(fonte.Provenienza.Ignoti, ignotoPercorsoGrezzo)
		} else {
			fonte.Provenienza.Ignoti = append(fonte.Provenienza.Ignoti, ignotoPercorsoVoce)
		}
	case contenitore != nil:
		return evidenze.Fonte{}, fmt.Errorf("estrazione: allegato %s: è dato un contenitore, ma il record non ne ha", a.ID)
	case a.PathInterno != nil && *a.PathInterno != "":
		return evidenze.Fonte{}, fmt.Errorf("estrazione: allegato %s: percorso interno senza contenitore", a.ID)
	default:
		// Un allegato diretto sta nel suo messaggio (allegato.messaggio_id): la fonte del contenitore è quella,
		// come dice la foglia per Fonte.ContenitoreID. Un ID nullo resta «ignoto».
		if a.MessaggioID != uuid.Nil {
			fonte.ContenitoreID = idFonte(fonteMessaggio, a.MessaggioID)
		}
		fonte.Provenienza.Ignoti = append(fonte.Provenienza.Ignoti, ignotoNomeDiretto)
	}

	rif, err := riferimentoDeiFatti(a, f)
	if err != nil {
		return evidenze.Fonte{}, err
	}
	fonte.RiferimentoFatti = rif
	return fonte, nil
}

// riferimentoDeiFatti: la terna dell'analisi, più ciò che la terna non garantisce, cioè il momento del calcolo
// e l'impronta del payload usato (v3 §2, §10.3). Senza fatti, «nessuno». Le sottoversioni le scrive la
// mappatura che legge la loro parte dei fatti.
func riferimentoDeiFatti(a fotorfq.Allegato, f *fotorfq.Fatti) (evidenze.RiferimentoFatti, error) {
	if f == nil {
		return evidenze.RiferimentoFatti{Tipo: riferimentoNessuno}, nil
	}
	if a.Sha256 != nil && *a.Sha256 != f.Sha256 {
		return evidenze.RiferimentoFatti{}, fmt.Errorf("estrazione: allegato %s: i fatti sono di un altro contenuto (sha256 %s, l'allegato ha %s)", a.ID, f.Sha256, *a.Sha256)
	}
	digest, err := fotorfq.ImprontaPayload(f.Payload)
	if err != nil {
		return evidenze.RiferimentoFatti{}, errors.Join(fmt.Errorf("estrazione: allegato %s", a.ID), err)
	}
	if f.Digest != "" && f.Digest != digest {
		return evidenze.RiferimentoFatti{}, fmt.Errorf("estrazione: allegato %s: Digest dei fatti %s, ma il payload ha l'impronta %s", a.ID, f.Digest, digest)
	}
	return evidenze.RiferimentoFatti{
		Tipo:                 riferimentoAnalisiFile,
		Sha256:               f.Sha256,
		VersioneAnalizzatore: int(f.Terna.Versione),
		HashConfigurazione:   f.Terna.HashConfigurazione,
		CalcolatoIl:          f.CalcolatoIl,
		DigestPayload:        digest,
	}, nil
}

// fonteDiMessaggio: la fonte di un messaggio. Il riferimento ai fatti è l'impronta dei testi acquisiti, con la
// presenza di ciascuno: un testo assente (nil) e un testo vuoto danno impronte diverse (E-21; REG §10.2).
// L'autore è l'indirizzo del mittente, se c'è; il canale "" vuol dire «non esportato». Oggetto e corpo sono
// quelli già trasformati dall'acquisizione (ingest.go:812), e la provenienza lo dichiara per tutti e due (5.4.5,
// «Mappatura mail»).
func fonteDiMessaggio(m fotorfq.Messaggio) (evidenze.Fonte, error) {
	fonte := evidenze.Fonte{
		ID:        idFonte(fonteMessaggio, m.ID),
		Tipo:      fonteMessaggio,
		OrigineID: m.ID,
		Provenienza: evidenze.Provenienza{
			Canale: m.Canale,
			Autore: m.MittenteIndirizzo,
			Data:   m.DataEvento,
			Ignoti: []string{ignotoOggettoGrezzo, ignotoCorpoGrezzo},
		},
	}
	if m.Canale == "" {
		fonte.Provenienza.Ignoti = append(fonte.Provenienza.Ignoti, ignotoCanale)
	}
	if m.MittenteIndirizzo == "" {
		fonte.Provenienza.Ignoti = append(fonte.Provenienza.Ignoti, ignotoAutoreMessaggio)
	}
	testi := struct {
		Oggetto    *string `json:"oggetto"`
		CorpoTesto *string `json:"corpo_testo"`
		CorpoHTML  *string `json:"corpo_html"`
	}{m.Oggetto, m.CorpoTesto, m.CorpoHTML}
	digest, err := jsoncanonico.ImprontaDi(testi)
	if err != nil {
		return evidenze.Fonte{}, errors.Join(fmt.Errorf("estrazione: messaggio %s: impronta dei testi", m.ID), err)
	}
	fonte.RiferimentoFatti = evidenze.RiferimentoFatti{Tipo: riferimentoMessaggio, DigestTesti: digest}
	return fonte, nil
}

// fonteDiTesto: la fonte sintetica di DaTesto. L'ID ha la forma delle altre, con l'UUID nullo.
func fonteDiTesto() evidenze.Fonte {
	return evidenze.Fonte{
		ID:               idFonte(fonteTesto, uuid.Nil),
		Tipo:             fonteTesto,
		OrigineID:        uuid.Nil,
		RiferimentoFatti: evidenze.RiferimentoFatti{Tipo: riferimentoNessuno},
	}
}
