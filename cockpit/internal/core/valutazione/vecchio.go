package valutazione

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// vecchio.go: il «vecchio» di ogni file (piano A, 6.4.6, LetturaRegistrata, LeggiCodiceRegistrato, VecchioLetto; R31 c,
// R42 B, R53 B; registro §10.2; T-B6-06: in P7c, perché nessun blocco prima li ha fatti). Il vecchio è ciò che c'è adesso
// nel DB per il file: la proposta attuale e, se il file è deciso, il documento confermato, con il codice letto dalla
// grammatica del cliente. Le scelte (quale proposta, quale documento, quale lettura) si fanno qui, una volta sola:
// confronto riceve i record piatti e non sceglie niente (6.4.6, «Il passaggio»).

// LetturaRegistrata: un codice scritto nel DB (proposta, documento, componente, identificativo) letto con la grammatica
// del cliente (6.4.6; R31 c).
//   - Originale: il codice com'è; Base: la base letta, normalizzata ("" se non si legge); Marcatore: il valore del
//     marcatore letto; Revisione: la revisione letta, normalizzata, solo se letta («» per un token sospeso).
//   - Leggibile: le forme dei selettori nell'ordine fisso (nome_file, cartiglio.codice, nodo_step.id, corpo) danno letture
//     complete che coprono tutto il codice, tutte con lo stesso namespace e la stessa base (leggiCodiceRegistrato).
//   - Motivo: perché non si legge (MotivoLettura*); "" se si legge.
type LetturaRegistrata struct {
	Originale string `json:"originale"`
	Base      string `json:"base"`
	Marcatore string `json:"marcatore"`
	Revisione string `json:"revisione"`
	Leggibile bool   `json:"leggibile"`
	Motivo    string `json:"motivo,omitempty"`
}

// I motivi di una lettura mancata (LetturaRegistrata.Motivo; fase 0, F.2 [T]).
const (
	MotivoLetturaCodiceVuoto     = "codice_vuoto"
	MotivoLetturaSenzaGrammatica = "senza_grammatica"
	MotivoLetturaNessunaLettura  = "nessuna_lettura"
	MotivoLetturaBasiDiverse     = "basi_diverse"
)

// LeggiCodiceRegistrato legge un codice registrato con la grammatica del cliente (6.4.6; R31 c): è l'involucro esportato
// della regola di B1 (leggiCodiceRegistrato, target.go), con lo stesso ordine dei selettori e il motivo di una lettura
// mancata. Mai un confronto per stringa (T-B1-07): un codice che non si legge non ha una base. m nil: nessuna grammatica.
func LeggiCodiceRegistrato(m *motorea.Motore, codice string) LetturaRegistrata {
	l, motivo := letturaDelCodiceRegistrato(m, codice)
	return letturaRegistrata(codice, l, motivo)
}

// letturaRegistrata: la lettura piatta di un codice, dalla forma letta e dal motivo.
func letturaRegistrata(codice string, l motorea.LetturaForma, motivo string) LetturaRegistrata {
	out := LetturaRegistrata{Originale: codice, Motivo: motivo}
	if motivo != "" {
		return out
	}
	out.Leggibile, out.Base, out.Marcatore = true, l.Base.Normalizzata, marcatoreDi(l)
	if l.Revisione != nil && l.Revisione.Stato == motorea.StatoRevisioneLetta {
		out.Revisione = l.Revisione.Normalizzata
	}
	return out
}

// vecchioLetto: ciò che c'è adesso nel DB per un file, con il codice letto dalla grammatica (6.4.6, VecchioLetto). È un
// tipo interno, non un campo dell'esito (F0-10): VecchioPiatto porta già tutto.
//   - Stato, Fonte: della proposta attuale; "" = nessuna proposta.
//   - Codice, Rev: della proposta, o del documento confermato se il file è deciso (registro §10.2).
//   - Lettura: il codice letto con la grammatica. revisione: la revisione vecchia interpretata, dal codice o dalla
//     colonna Rev, con la lettura da confrontare con la grammatica (ConfrontaRevisioni) o il motivo per cui non si
//     confronta (revisioneVecchia; R113 B).
//   - CodiceLetto: la lettura del vecchio motore, dettagli.valutazione.codice della proposta (T-B0-14, F0-02);
//     CodiceLettoBase: la sua base, letta con la stessa grammatica (F0-19); CodiceLettoMarcatore: il marcatore della
//     stessa lettura (LetturaForma.Marcatore), vuoto se non c'è o se il codice non si legge (il gemello per R114,
//     precisata dall'utente il 07/10).
//   - Componente, Documento, SostituitoDa: dal documento confermato, mai dalla proposta (registro §10.2).
//   - ComponenteProposta, DecisoIl, Destinazione: della proposta («assegna», la decisione, le chiavi F8).
type vecchioLetto struct {
	Stato, Fonte         string
	Codice, Rev          string
	Lettura              LetturaRegistrata
	revisione            revisioneVecchia
	CodiceLetto          string
	CodiceLettoBase      string
	CodiceLettoMarcatore string
	Componente           *uuid.UUID
	Documento            *uuid.UUID
	ComponenteProposta   *uuid.UUID
	SostituitoDa         *uuid.UUID
	DecisoIl             *time.Time
	Destinazione         []string
}

// vecchioDelThread: le proposte e i documenti del thread per allegato, scelti una volta: la proposta di un allegato è
// una sola (documento_proposta.allegato_id è UNIQUE, 0001); con due righe per lo stesso allegato vince la prima in ordine
// di ID. Il documento di un allegato è quello che lo porta in documento_provenienza: prima i correnti, poi gli altri, in
// ordine di ID (come il tipo documentale, T-B4-31).
type vecchioDelThread struct {
	m         *motorea.Motore
	proposte  map[uuid.UUID]fotorfq.PropostaAttuale
	documenti map[uuid.UUID]fotorfq.DocumentoConfermato
}

func nuovoVecchioDelThread(t fotorfq.Thread, m *motorea.Motore) *vecchioDelThread {
	v := &vecchioDelThread{m: m, proposte: map[uuid.UUID]fotorfq.PropostaAttuale{}, documenti: map[uuid.UUID]fotorfq.DocumentoConfermato{}}
	proposte := append([]fotorfq.PropostaAttuale(nil), t.Proposte...)
	sort.SliceStable(proposte, func(i, j int) bool { return proposte[i].ID.String() < proposte[j].ID.String() })
	for _, p := range proposte {
		if _, ok := v.proposte[p.AllegatoID]; !ok {
			v.proposte[p.AllegatoID] = p
		}
	}
	docs := append([]fotorfq.DocumentoConfermato(nil), t.Documenti...)
	sort.SliceStable(docs, func(i, j int) bool {
		if (docs[i].SostituitoDa == nil) != (docs[j].SostituitoDa == nil) {
			return docs[i].SostituitoDa == nil
		}
		return docs[i].ID.String() < docs[j].ID.String()
	})
	for _, d := range docs {
		for _, a := range d.Allegati {
			if _, ok := v.documenti[a]; !ok {
				v.documenti[a] = d
			}
		}
	}
	return v
}

// delFile: il vecchio di un allegato. Il file è deciso se un documento confermato lo porta: allora codice e revisione
// sono quelli del documento, anche vuoti (registro §10.2), altrimenti quelli della proposta.
func (v *vecchioDelThread) delFile(allegato uuid.UUID) vecchioLetto {
	var out vecchioLetto
	p, conProposta := v.proposte[allegato]
	if conProposta {
		out.Stato, out.Fonte = p.Stato, p.Fonte
		out.Codice, out.Rev = testoDi(p.Codice), testoDi(p.Rev)
		out.ComponenteProposta = copiaUUID(p.ComponenteID)
		if p.DecisoIl != nil {
			il := p.DecisoIl.UTC().Truncate(time.Millisecond)
			out.DecisoIl = &il
		}
		out.CodiceLetto, out.Destinazione = leggiDettagli(p.Dettagli)
	}
	if d, deciso := v.documenti[allegato]; deciso {
		out.Codice, out.Rev = testoDi(d.Codice), testoDi(d.Rev)
		id := d.ID
		out.Componente, out.Documento, out.SostituitoDa = copiaUUID(d.ComponenteID), &id, copiaUUID(d.SostituitoDa)
	}
	letto := LeggiCodiceRegistrato(v.m, out.CodiceLetto)
	out.CodiceLettoBase, out.CodiceLettoMarcatore = letto.Base, letto.Marcatore
	l, motivo := letturaDelCodiceRegistrato(v.m, out.Codice)
	out.Lettura = letturaRegistrata(out.Codice, l, motivo)
	var forma *motorea.LetturaForma
	if motivo == "" {
		forma = &l
	}
	out.revisione = leggiRevisioneVecchia(v.m, forma, out.Rev)
	return out
}

// revisioneVecchia: la revisione vecchia di un file, interpretata (R113 B ratificata: valore originale, interpretazione e
// provenienza; E2 §2.6). valore e da vanno in VecchioPiatto.Revisione e RevisioneDa; rev è la lettura da confrontare con
// quella del file; motivo è il motivo di non_confrontabili dal lato vecchio, "" se rev c'è.
type revisioneVecchia struct {
	valore string
	da     string
	rev    *motorea.RevisioneLetta
	motivo string
}

// leggiRevisioneVecchia: la revisione vecchia dal codice letto (forma, nil se il codice non si legge) e dalla colonna rev
// della riga che dà il codice (la proposta o il documento), letta con motorea.LeggiRevisioneRegistrata e con la famiglia
// della lettura del codice: un codice che non si legge non ha una famiglia, e la colonna resta non determinabile. Il
// primo caso che vale:
//   - codice non letto, o con una revisione non «letta» (un token sospeso): nessuna revisione, revisione_vecchia_non_letta,
//     qualunque cosa dica la colonna (il codice ha già una revisione, che non si legge: la colonna non la sostituisce);
//   - codice con la revisione letta: quella, da «codice». Se la colonna si legge con la regola della famiglia e non
//     concorda (ConfrontaRevisioni discordante), il confronto non si fa: revisione_vecchia_discorde, senza scegliere. Una
//     colonna che non si legge non contraddice il codice;
//   - codice senza revisione e colonna vuota: revisione_vecchia_non_letta;
//   - codice senza revisione e colonna letta: quella, da «colonna»;
//   - codice senza revisione e colonna che non si legge: un motivo per lo stato della lettura (T-B6-202).
//
// La colonna si legge senza gli spazi ai bordi, come il codice registrato (letturaDelCodiceRegistrato); l'originale resta
// nel record piatto (Rev). Mai «codice + rev» composti: la colonna si legge da sola, con la regola della revisione.
func leggiRevisioneVecchia(m *motorea.Motore, forma *motorea.LetturaForma, rev string) revisioneVecchia {
	if forma == nil || (forma.Revisione != nil && forma.Revisione.Stato != motorea.StatoRevisioneLetta) {
		return revisioneVecchia{motivo: MotivoRevisioniVecchiaNonLetta}
	}
	colonna := strings.TrimSpace(rev)
	var registrata *motorea.RevisioneRegistrata
	if colonna != "" {
		r := m.LeggiRevisioneRegistrata(forma.Famiglia, colonna)
		registrata = &r
	}
	letta := registrata != nil && registrata.Stato == motorea.StatoRevisioneRegistrataLetta && registrata.Revisione != nil
	if forma.Revisione != nil {
		out := revisioneVecchia{valore: forma.Revisione.Normalizzata, da: RevisioneDaCodice, rev: forma.Revisione}
		if letta && motorea.ConfrontaRevisioni(*forma.Revisione, *registrata.Revisione) == motorea.CompatibilitaDiscordante {
			out.rev, out.motivo = nil, MotivoRevisioniVecchiaDiscorde
		}
		return out
	}
	switch {
	case registrata == nil:
		return revisioneVecchia{motivo: MotivoRevisioniVecchiaNonLetta}
	case letta:
		return revisioneVecchia{valore: registrata.Revisione.Normalizzata, da: RevisioneDaColonna, rev: registrata.Revisione}
	case registrata.Stato == motorea.StatoRevisioneRegistrataAmbigua:
		return revisioneVecchia{motivo: MotivoRevisioniColonnaAmbigua}
	case registrata.Stato == motorea.StatoRevisioneRegistrataNonInterpretabile:
		return revisioneVecchia{motivo: MotivoRevisioniColonnaNonInterpretabile}
	}
	// nessuna_regola, e ogni stato che motorea aggiungesse: la colonna resta solo colonna, mai letta per stringa
	return revisioneVecchia{motivo: MotivoRevisioniSoloInColonna}
}

// regolaOperatoreLegacy: la regola con cui il legacy scrive in dettagli.valutazione la decisione di una persona
// (classificazione.RegolaOperatore, ripetuta qui perché valutazione non importa il motore legacy). Quella non è una
// lettura del vecchio motore. Se cambia, lo dicono le prove del pacchetto.
const regolaOperatoreLegacy = "operatore"

// Le versioni di dettagli.valutazione che si conoscono (classificazione.VersioneValutazione e la v1 di prima, ripetute
// qui perché valutazione non importa il motore legacy; R-64).
const (
	versioneValutazioneV1Legacy = 1
	versioneValutazioneLegacy   = 2
)

// leggiDettagli: dai dettagli della proposta la lettura del vecchio motore e le chiavi della destinazione F8.
//   - CodiceLetto: dettagli.valutazione.codice (T-B0-14; F0-02: il nome del campo resta quello del piano, la fonte è
//     quella del contratto, perché dettagli.codice_letto non c'è nei dati). La dimensione del codice ha il valore in
//     «valore»; una dimensione con la regola «operatore» è la decisione di una persona, non la lettura del vecchio
//     motore, e lascia CodiceLetto vuoto (dubbio T-B6-22). Una forma che non si conosce, anche: la lettura non si inventa.
//     Si conoscono solo le versioni 1 e 2 di dettagli.valutazione (v), come LeggiValutazione del legacy (R-64 della
//     revisione di V1); un'altra versione, o nessuna, lascia CodiceLetto vuoto. Per la v1 vale il valore registrato
//     allora, cioè la lettura del vecchio motore di quel giorno: il legacy ricompone le dimensioni di una riga v1, e qui
//     una regola del legacy non si rifà.
//   - Destinazione: le «chiave» dei candidati di dettagli.destinazione, nell'ordine scritto, senza doppioni; nil se la
//     destinazione non c'è, e sempre negli export, che non hanno i dettagli.
func leggiDettagli(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 {
		return "", nil
	}
	var d struct {
		Valutazione *struct {
			V      int             `json:"v"`
			Codice json.RawMessage `json:"codice"`
		} `json:"valutazione"`
		Destinazione *struct {
			Candidati []struct {
				Chiave string `json:"chiave"`
			} `json:"candidati"`
		} `json:"destinazione"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return "", nil
	}
	codice := ""
	if d.Valutazione != nil && (d.Valutazione.V == versioneValutazioneV1Legacy || d.Valutazione.V == versioneValutazioneLegacy) &&
		len(d.Valutazione.Codice) > 0 {
		var dim struct {
			Valore string `json:"valore"`
			Regola string `json:"regola"`
		}
		if json.Unmarshal(d.Valutazione.Codice, &dim) == nil && dim.Regola != regolaOperatoreLegacy {
			codice = dim.Valore
		}
	}
	var dest []string
	if d.Destinazione != nil {
		visti := map[string]bool{}
		for _, c := range d.Destinazione.Candidati {
			if c.Chiave != "" && !visti[c.Chiave] {
				visti[c.Chiave] = true
				dest = append(dest, c.Chiave)
			}
		}
	}
	return codice, dest
}

// diagnosticaCodiceNonLeggibile: confronto.codice_registrato_non_leggibile per il codice registrato di un allegato che la
// grammatica del cliente non legge (6.4.6, LetturaRegistrata; R31 c). Solo con la grammatica e un codice non vuoto:
// senza grammatica il thread non si valuta, e lo dice il suo motivo. Il messaggio non riporta il codice: può finire nel
// log.
func diagnosticaCodiceNonLeggibile(allegato uuid.UUID, l LetturaRegistrata) evidenze.Diagnostica {
	return evidenze.Diagnostica{
		Codice:    CodiceCodiceRegistratoNonLeggibile,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaDati,
		Percorso:  "confrontabili[" + allegato.String() + "].vecchio",
		Messaggio: "il codice registrato del file non si legge con la grammatica del cliente (" + l.Motivo + "): la base vecchia resta vuota, nessun confronto per stringa",
		Rif:       []string{allegato.String()},
	}
}

// vecchiProdotti: il vecchio dei prodotti del thread (par.3.3.8, VecchiProdotti; F0-18): «identificativo:<codice>» per
// ogni identificativo confermato, «candidato:<codice>» per ogni candidato di codice della fotografia (il caricatore non
// porta quelli dell'agente, e ValidaFotografia li rifiuta), in ordine di byte, senza doppioni. I codici sono com'è nel DB:
// è il vecchio, non si legge.
func vecchiProdotti(t fotorfq.Thread) []string {
	visti := map[string]bool{}
	var out []string
	aggiungi := func(s string) {
		if !visti[s] {
			visti[s] = true
			out = append(out, s)
		}
	}
	for _, id := range t.Identificativi {
		if id.ConfermatoDa != nil && strings.TrimSpace(id.Codice) != "" {
			aggiungi(rifIdentificativo + id.Codice)
		}
	}
	for _, c := range t.CandidatiCodice {
		if strings.TrimSpace(c.Codice) != "" {
			aggiungi(prefissoVecchioCandidato + c.Codice)
		}
	}
	sort.Strings(out)
	return out
}

// prefissoVecchioCandidato: il prefisso di un candidato di codice in EsitoThread.VecchiProdotti (F0-18).
const prefissoVecchioCandidato = "candidato:"
