package motore_a

import (
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	valut "promatec/cockpit/internal/core/valutazione"
)

// passaggio.go: la copia di app/bancoa/passaggio.go, con i tre aiuti di app/bancoa/banco.go (copiaUUID, copiaTempo,
// copiaStringhe), per la Distinta nuova del motore A (giro 5, A1d-P2; piano P2-P3; derivato dalla PR #18 di Stefano
// Valo, c543e89: motore_a_passaggio.go). transport non importa app (internal/README.md) e spostare il passaggio in un
// package comune toccherebbe A1c e il banco: si copia, e la duplicazione è dichiarata.
//
// I nomi delle funzioni, l'alias valut e i corpi sono quelli del banco, byte per byte: la prova gemella di A1c-L1-31
// (passaggio_test.go, nel QA) legge i due sorgenti con go/parser e confronta ogni funzione, e confronta i byte JSON della
// sorgente e della copia con ogni campo riempito. Un campo nuovo da una parte si aggiunge nelle due copie.
//
// Il passaggio dai record piatti di valutazione ai DTO di confronto: i due pacchetti non si importano, chi chiama copia
// campo per campo, senza scelte. Le scelte (quali basi, quale lettura del vecchio, se le revisioni sono confrontabili)
// le ha già fatte valutazione, una volta sola.

// inFile: un allegato del thread, il vecchio e il nuovo.
func inFile(f valut.FileConfrontabile) confronto.File {
	return confronto.File{
		AllegatoID: f.AllegatoID,
		Vecchio:    inVecchio(f.Vecchio),
		Nuovo:      inNuovo(f.Nuovo),
	}
}

// inFiles: tutti gli allegati del thread, nell'ordine di valutazione (confronto li riordina per AllegatoID).
func inFiles(ff []valut.FileConfrontabile) []confronto.File {
	out := make([]confronto.File, 0, len(ff))
	for _, f := range ff {
		out = append(out, inFile(f))
	}
	return out
}

// inVecchio: il vecchio di un allegato, con i 19 campi di CP.2: CodiceLettoBase subito dopo CodiceLetto (F0-19),
// CodiceLettoMarcatore subito dopo CodiceLettoBase (il gemello di R114: il marcatore della lettura del vecchio motore,
// che la misura conta a parte in PrimaMarcatore e MarcatoreSoloDaUnLato; senza la copia un marcatore del solo codice
// deciso conterebbe come «da un lato solo») e RevisioneDa subito dopo CodiceLettoMarcatore (il gemello di R113 B
// ratificata, E2 §2.6: da dove viene la revisione vecchia interpretata, «codice» o «colonna»; senza la copia
// l'indicatore di revisione perderebbe la provenienza, e il banco non dividerebbe per stato la colonna letta).
func inVecchio(v valut.VecchioPiatto) confronto.Vecchio {
	return confronto.Vecchio{
		Stato:                v.Stato,
		Fonte:                v.Fonte,
		Codice:               v.Codice,
		Rev:                  v.Rev,
		Base:                 v.Base,
		Marcatore:            v.Marcatore,
		Revisione:            v.Revisione,
		Leggibile:            v.Leggibile,
		MotivoLettura:        v.MotivoLettura,
		CodiceLetto:          v.CodiceLetto,
		CodiceLettoBase:      v.CodiceLettoBase,
		CodiceLettoMarcatore: v.CodiceLettoMarcatore,
		RevisioneDa:          v.RevisioneDa,
		Componente:           copiaUUID(v.Componente),
		Documento:            copiaUUID(v.Documento),
		ComponenteProposta:   copiaUUID(v.ComponenteProposta),
		SostituitoDa:         copiaUUID(v.SostituitoDa),
		DecisoIl:             copiaTempo(v.DecisoIl),
		Destinazione:         copiaStringhe(v.Destinazione),
	}
}

// inNuovo: il nuovo di un allegato.
func inNuovo(n valut.NuovoPiatto) confronto.Nuovo {
	out := confronto.Nuovo{
		Valutato:        n.Valutato,
		Motivo:          n.Motivo,
		Basi:            copiaStringhe(n.Basi),
		Collocazione:    n.Collocazione,
		Associazione:    n.Associazione,
		Disponibilita:   n.Disponibilita,
		Revisione:       n.Revisione,
		Revisioni:       n.Revisioni,
		MotivoRevisioni: n.MotivoRevisioni,
	}
	if n.Candidati != nil {
		out.Candidati = make([]confronto.Candidato, 0, len(n.Candidati))
		for _, c := range n.Candidati {
			out.Candidati = append(out.Candidati, inCandidato(c))
		}
	}
	return out
}

// inCandidato: un candidato di ancoraggio.
func inCandidato(c valut.CandidatoPiatto) confronto.Candidato {
	return confronto.Candidato{
		Target:   c.Target,
		Livello:  c.Livello,
		Base:     c.Base,
		Autorita: c.Autorita,
		Radici:   copiaStringhe(c.Radici),
	}
}

// inProdotti: i candidati prodotto della mail, nell'ordine di valutazione.
func inProdotti(pp []valut.ProdottoConfrontabile) []confronto.ProdottoNuovo {
	out := make([]confronto.ProdottoNuovo, 0, len(pp))
	for _, p := range pp {
		out = append(out, inProdotto(p))
	}
	return out
}

// inProdotto: un candidato prodotto.
func inProdotto(p valut.ProdottoConfrontabile) confronto.ProdottoNuovo {
	var q *int
	if p.Quantita != nil {
		v := *p.Quantita
		q = &v
	}
	return confronto.ProdottoNuovo{
		CodiceRichiesto: p.CodiceRichiesto,
		Base:            p.Base,
		Fase:            p.Fase,
		Quantita:        q,
		QuantitaDaCella: p.QuantitaDaCella,
	}
}

// ---- le copie dei campi, come in app/bancoa/banco.go ----

func copiaUUID(u *uuid.UUID) *uuid.UUID {
	if u == nil {
		return nil
	}
	v := *u
	return &v
}

func copiaTempo(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

func copiaStringhe(s []string) []string {
	if s == nil {
		return nil
	}
	return append([]string{}, s...)
}
