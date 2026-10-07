package motorea

import "sort"

// VersioneRevisioneRegistrata: l'algoritmo che legge una revisione registrata a parte, fuori da ogni testo con un
// selettore (A1c, B6b; R113 B ratificata, emendamento E2 §2.6). È una versione propria, accanto a VersioneAlgoritmo e
// a VersioneComposizione, che non cambiano: il metodo non tocca il riconoscimento, l'interpretazione né il compositore,
// e la valutazione la mette nell'impronta dell'esito. Cambiare le regole di questo file (quali regole valgono, l'ordine
// degli stati, i motivi) vuol dire cambiarla, con un commit che lo dichiara.
const VersioneRevisioneRegistrata = "revisione-registrata-1"

// RevisioneRegistrata: una revisione scritta a parte nel DB (la colonna rev di una proposta o di un documento del vecchio
// motore), letta con la regola di revisione in campo separato della famiglia (R113 B: «conserva valore originale,
// interpretazione e provenienza della revisione legacy»). È un valore calcolato, mai una decisione: non cambia la riga da
// cui viene, e chi lo usa lo tiene accanto all'originale, mai al suo posto.
//   - Originale: il testo ricevuto, com'è.
//   - Revisione: la lettura della regola quando la regola legge il testo per intero (Stato «letta», oppure un token
//     sospeso, «non_interpretabile» con il token conservato, D5); nil altrimenti.
//   - Stato: letta | non_interpretabile | nessuna_regola | ambigua (StatoRevisioneRegistrata*).
//   - Regola: «famiglia/regola» della regola applicata, quando è una sola; "" per nessuna_regola e ambigua, e per più
//     regole riservate, perché nessuna si sceglie.
//   - Motivo: perché lo stato non è «letta» (MotivoRevisioneRegistrata*); "" quando è letta.
type RevisioneRegistrata struct {
	Originale string          `json:"originale"`
	Revisione *RevisioneLetta `json:"revisione,omitempty"`
	Stato     string          `json:"stato"`
	Regola    string          `json:"regola,omitempty"`
	Motivo    string          `json:"motivo,omitempty"`
}

// Gli stati di una RevisioneRegistrata (R113 B; E2 §2.6). Non sono gli stati di RevisioneLetta: quelli dicono che cosa
// ha letto una regola, questi anche se una regola c'era e se era una sola.
const (
	StatoRevisioneRegistrataLetta             = "letta"
	StatoRevisioneRegistrataNonInterpretabile = "non_interpretabile"
	StatoRevisioneRegistrataNessunaRegola     = "nessuna_regola"
	StatoRevisioneRegistrataAmbigua           = "ambigua"
)

// I motivi di una RevisioneRegistrata che non è «letta» [T]. Non sono codici di diagnostica: il motivo sta nella
// RevisioneRegistrata, come per il CodiceComposto (T-15), e motorea non ha codici nuovi per R113.
const (
	// nessuna_regola: la famiglia non ha regole di revisione in campo separato, né attive né riservate (anche una
	// famiglia vuota o che il motore non conosce, e un motore nullo).
	MotivoRevisioneRegistrataRegoleAssenti = "regole_assenti"
	// ambigua: la famiglia ha più regole attive in campo separato, con ID diversi: nessuna si sceglie.
	MotivoRevisioneRegistrataRegoleMultiple = "regole_multiple"
	// non_interpretabile: la famiglia ha solo regole riservate (Q1): l'originale si conserva, nessun valore.
	MotivoRevisioneRegistrataRegolaRiservata = "regola_riservata"
	// non_interpretabile: il testo è vuoto, quindi non c'è niente da leggere; chi chiama non dovrebbe chiedere.
	MotivoRevisioneRegistrataTestoVuoto = "testo_vuoto"
	// non_interpretabile: la regola non legge il testo per intero (un pezzo di codice, un testo in più).
	MotivoRevisioneRegistrataTestoNonLetto = "testo_non_letto"
	// non_interpretabile: la regola legge il testo come un token sospeso (D5): conservato, senza valore.
	MotivoRevisioneRegistrataTokenSospeso = "token_sospeso"
)

// LeggiRevisioneRegistrata legge una revisione registrata a parte (per esempio la colonna rev del vecchio motore) con le
// regole di revisione in campo separato della famiglia data (R113 B; E2 §2.6). È un metodo nuovo, come
// ComponiCodiceDocumentale (T-B0-17): non cambia nessuna firma, nessun campo e nessun comportamento di A1a, A1b e del
// compositore (D2), e VersioneAlgoritmo resta motorea-1.
//
// Le regole, nell'ordine:
//   - valgono le sole regole della famiglia con sorgente campo_separato, senza doppioni per ID, prese da tutti i
//     selettori: il testo registrato non ha un selettore, quindi nessun selettore sceglie la regola. Sono le regole
//     compilate nel motore (attive, con almeno un selettore attivo), più le riservate per lo stato;
//   - nessuna regola, né attiva né riservata: nessuna_regola. Il default prudente di C-34 qui non si rifà: il testo non
//     è un campo del cartiglio, e lo stato dice che la grammatica non ha una regola per leggerlo;
//   - più regole attive con ID diversi: ambigua, senza provarle (come per un campo senza letture nell'entità in
//     Interpreta: nessuna si sceglie);
//   - nessuna regola attiva e almeno una riservata: non_interpretabile (Q1), come in Interpreta, dove una riservata conta
//     solo per una famiglia che non ne ha di attive;
//   - una regola attiva: si applica al testo intero (D-07), come la revisione in campo separato di Interpreta (la stessa
//     regex compilata). La legge: letta, con la RevisioneLetta; la legge come token sospeso: non_interpretabile, con il
//     token conservato; non la legge per intero, o il testo è vuoto: non_interpretabile, senza valore.
//
// Mai «codice + rev» composti: il testo si legge da solo, con la regola della revisione, e mai si unisce al codice per
// leggerlo con una forma. Il testo non si ripulisce: chi chiama toglie gli spazi ai bordi, se vuole, come fa per il codice
// registrato. Il confronto con altre revisioni resta di ConfrontaRevisioni (solo le equivalenze dichiarate): qui nessuna
// rappresentazione diversa diventa equivalente.
//
// È deterministica: niente orologio, file, rete, mappe in ordine; lo stesso motore, la stessa famiglia e lo stesso testo
// danno la stessa RevisioneRegistrata. Un motore nullo non ha regole.
func (m *Motore) LeggiRevisioneRegistrata(famiglia, testo string) RevisioneRegistrata {
	out := RevisioneRegistrata{Originale: testo}
	attive, riservate := m.regoleRegistrate(famiglia)
	switch {
	case len(attive) > 1:
		out.Stato, out.Motivo = StatoRevisioneRegistrataAmbigua, MotivoRevisioneRegistrataRegoleMultiple
		return out
	case len(attive) == 0 && len(riservate) == 0:
		out.Stato, out.Motivo = StatoRevisioneRegistrataNessunaRegola, MotivoRevisioneRegistrataRegoleAssenti
		return out
	case len(attive) == 0:
		out.Stato, out.Motivo = StatoRevisioneRegistrataNonInterpretabile, MotivoRevisioneRegistrataRegolaRiservata
		if len(riservate) == 1 {
			out.Regola = famiglia + "/" + riservate[0]
		}
		return out
	}
	p := attive[0]
	out.Regola = famiglia + "/" + p.rev.id
	out.Stato = StatoRevisioneRegistrataNonInterpretabile
	if testo == "" {
		out.Motivo = MotivoRevisioneRegistrataTestoVuoto
		return out
	}
	rev := p.leggi(testo)
	switch {
	case rev == nil:
		out.Motivo = MotivoRevisioneRegistrataTestoNonLetto
	case rev.Stato != StatoRevisioneLetta:
		out.Revisione, out.Motivo = rev, MotivoRevisioneRegistrataTokenSospeso
	default:
		out.Revisione, out.Stato = rev, StatoRevisioneRegistrataLetta
	}
	return out
}

// regoleRegistrate: le regole di revisione in campo separato della famiglia, da tutti i selettori, senza doppioni per ID
// e in ordine di ID: i piani attivi compilati nel motore e gli ID delle riservate. Le mappe del motore si leggono solo
// per cercare: l'ordine viene dall'ID, mai dalla mappa.
func (m *Motore) regoleRegistrate(famiglia string) ([]*pianoRevisione, []string) {
	if m == nil || famiglia == "" {
		return nil, nil
	}
	perID := map[string]*pianoRevisione{}
	for _, piani := range m.revisioniCampo {
		for _, p := range piani {
			if p.famiglia == famiglia {
				if _, visto := perID[p.rev.id]; !visto {
					perID[p.rev.id] = p
				}
			}
		}
	}
	attive := make([]*pianoRevisione, 0, len(perID))
	for _, p := range perID {
		attive = append(attive, p)
	}
	sort.Slice(attive, func(i, j int) bool { return attive[i].rev.id < attive[j].rev.id })

	viste := map[string]bool{}
	var riservate []string
	for _, rr := range m.regole.riservate {
		for _, r := range rr {
			if r.famiglia == famiglia && !viste[r.regola] {
				viste[r.regola] = true
				riservate = append(riservate, r.regola)
			}
		}
	}
	sort.Strings(riservate)
	return attive, riservate
}
