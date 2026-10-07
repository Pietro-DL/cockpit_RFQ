package bancoa

import (
	"promatec/cockpit/internal/core/confronto"
	valut "promatec/cockpit/internal/core/valutazione"
)

// passaggio.go: il passaggio dai record piatti di valutazione ai DTO di confronto (piano A, 6.4.6, «Il passaggio»;
// R42 B, R53 B; fase 0 di B6, CP.1–CP.3). I due pacchetti non si importano: chi chiama copia campo per campo, senza
// scelte. Le scelte (quali basi, quale lettura del vecchio, se le revisioni sono confrontabili) le ha già fatte
// valutazione, una volta sola.
//
// In compilazione nessuno lega i due insiemi di tipi: la guardia è la prova strutturale A1c-L1-31
// (passaggio_test.go), che confronta nomi, ordine, tipi e tag dei campi con reflect e poi i byte JSON della sorgente e
// della copia. Un campo nuovo da una parte si aggiunge qui, nella funzione del suo tipo.

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

// inVecchio: il vecchio di un allegato, con i 18 campi di CP.2: CodiceLettoBase subito dopo CodiceLetto (F0-19) e
// CodiceLettoMarcatore subito dopo CodiceLettoBase (il gemello di R114: il marcatore della lettura del vecchio motore,
// che la misura conta a parte in PrimaMarcatore e MarcatoreSoloDaUnLato; senza la copia un marcatore del solo codice
// deciso conterebbe come «da un lato solo»).
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
