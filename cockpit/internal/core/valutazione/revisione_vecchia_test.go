// L1 — la revisione del vecchio motore, interpretata (R113 B ratificata; emendamento E2 §2.6; A1c, B6b; T-B6-201,
// T-B6-202): la colonna rev della riga che dà il codice (la proposta o il documento) si legge con la regola in campo
// separato della famiglia del codice (motorea.LeggiRevisioneRegistrata), solo se il codice si legge senza revisione; il
// record piatto porta il valore interpretato e la provenienza (RevisioneDa «codice», «colonna» o ""); la revisione del
// codice e quella della colonna discordi danno non_confrontabili con revisione_vecchia_discorde, senza scegliere; una
// colonna che non si legge ha un motivo per stato (nessuna regola, non interpretabile, ambigua); senza la famiglia (il
// codice che non si legge) la colonna non si legge; la versione della lettura nell'esito. Aggiorna A1c-L1-30 per la parte
// delle revisioni; la forma dei campi (19 nel vecchio) è in esito_test.go e, nel chiamante, in A1c-L1-31.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
// Le prove citano i requisiti (R113, E2 §2.6), mai i casi degli attesi.
package valutazione_test

import (
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// regolaColonnaACME: la regola di revisione in campo separato della catena, sul campo della revisione del cartiglio:
// una cifra, come la revisione in linea della famiglia.
func regolaColonnaACME(id, pattern string) grammatica.RegolaRevisione {
	return grammatica.RegolaRevisione{ID: id, Selettori: []string{"cartiglio.revisione"}, Stato: grammatica.StatoAttiva,
		Sorgente: grammatica.SorgenteCampoSeparato,
		Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: pattern, Significato: grammatica.SignificatoNessuno}}}
}

// famigliaCatenaColonna: la famiglia della catena con le regole in campo separato date, accanto a quella in linea.
func famigliaCatenaColonna(regole ...grammatica.RegolaRevisione) grammatica.FamigliaCodice {
	f := famigliaCatena()
	f.Revisioni = append(f.Revisioni, regole...)
	return f
}

// insiemeColonna: l'insieme delle regole ACME con la famiglia data, dalla porta del prodotto.
func insiemeColonna(t *testing.T, f grammatica.FamigliaCodice) *motorea.InsiemeRegole {
	t.Helper()
	return insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", f)})
}

// vecchioColonna: il vecchio e il nuovo del 2D deciso della scena del vecchio, con il codice e la colonna rev del
// documento dati (rev "" = colonna assente). Il file legge la revisione 1 nel cartiglio.
func vecchioColonna(t *testing.T, r *motorea.InsiemeRegole, codice, rev string) (valutazione.VecchioPiatto, valutazione.NuovoPiatto) {
	t.Helper()
	th := scenaVecchio(t, codice, `{"valutazione": {"v": 2, "codice": {"valore": "7120299A1", "regola": "cartiglio"}}}`)
	for i := range th.Documenti {
		if th.Documenti[i].ID == dDisegno {
			th.Documenti[i].Rev = nil
			if rev != "" {
				th.Documenti[i].Rev = testo(rev)
			}
		}
	}
	f := confrontabileDi(t, esitoThread(t, calcola(t, fotografiaDi(th), r, valutazione.Ingressi{}), threadACME), aDisegno)
	if f.Nuovo.Revisione != "1" {
		t.Fatalf("premessa: il file legge la revisione %q, attesa 1", f.Nuovo.Revisione)
	}
	return f.Vecchio, f.Nuovo
}

// casoColonna: un caso delle prove della colonna, con il vecchio e il confronto attesi.
type casoColonna struct {
	nome, codice, rev string
	revisione, da     string
	revisioni, motivo string
	insieme           *motorea.InsiemeRegole
}

func controllaColonna(t *testing.T, c casoColonna) {
	t.Helper()
	v, n := vecchioColonna(t, c.insieme, c.codice, c.rev)
	if v.Revisione != c.revisione || v.RevisioneDa != c.da || n.Revisioni != c.revisioni || n.MotivoRevisioni != c.motivo {
		t.Errorf("%s (codice %q, rev %q): vecchio %q da %q, revisioni %q/%q; attesi %q da %q, %q/%q", c.nome, c.codice, c.rev,
			v.Revisione, v.RevisioneDa, n.Revisioni, n.MotivoRevisioni, c.revisione, c.da, c.revisioni, c.motivo)
	}
	// l'originale resta: il codice e la colonna come sono nel DB
	if v.Codice != c.codice || v.Rev != c.rev {
		t.Errorf("%s: originale %q e %q, attesi %q e %q", c.nome, v.Codice, v.Rev, c.codice, c.rev)
	}
	// Revisione vuota se e solo se RevisioneDa è vuoto (T-B6-201)
	if (v.Revisione == "") != (v.RevisioneDa == "") {
		t.Errorf("%s: revisione %q con la provenienza %q", c.nome, v.Revisione, v.RevisioneDa)
	}
}

// TestR113LaColonnaLettaConLaRegolaDellaFamiglia (R113 B; E2 §2.6): il codice letto senza revisione e la colonna letta
// con la regola in campo separato della famiglia: la revisione vecchia viene dalla colonna, e si confronta con quella del
// file (uguali, diverse), senza gli spazi ai bordi; con la revisione nel codice vale quella, da «codice»; senza la
// colonna, o con il codice che non si legge, la revisione vecchia non c'è. Un codice con la revisione scritta come token
// sospeso ha una revisione che non si legge, e la colonna non la sostituisce (T-B6-206).
func TestR113LaColonnaLettaConLaRegolaDellaFamiglia(t *testing.T) {
	r := insiemeColonna(t, famigliaCatenaColonna(regolaColonnaACME("rev-colonna", "[0-9]")))
	nc, rnl := valutazione.RevisioniNonConfrontabili, valutazione.MotivoRevisioniVecchiaNonLetta
	conToken := famigliaCatenaColonna(regolaColonnaACME("rev-colonna", "[0-9]"))
	conToken.Revisioni[0].TokenSospesi = []grammatica.TokenSospeso{{ID: "Q-ACME-3", Pattern: "x", Riserva: "Q-ACME-3"}}
	controllaColonna(t, casoColonna{nome: "il codice con un token sospeso e la colonna letta", codice: "7120200Ax", rev: "1", revisioni: nc,
		motivo: rnl, insieme: insiemeColonna(t, conToken)})
	for _, c := range []casoColonna{
		{nome: "la colonna uguale al file", codice: "7120200A", rev: "1", revisione: "1", da: valutazione.RevisioneDaColonna, revisioni: valutazione.RevisioniUguali},
		{nome: "la colonna diversa dal file", codice: "7120200A", rev: "2", revisione: "2", da: valutazione.RevisioneDaColonna, revisioni: valutazione.RevisioniDiverse},
		{nome: "la colonna con gli spazi ai bordi", codice: "7120200A", rev: " 2 ", revisione: "2", da: valutazione.RevisioneDaColonna, revisioni: valutazione.RevisioniDiverse},
		{nome: "il codice con la revisione, senza colonna", codice: "7120200A2", rev: "", revisione: "2", da: valutazione.RevisioneDaCodice, revisioni: valutazione.RevisioniDiverse},
		{nome: "il codice e la colonna concordi", codice: "7120200A1", rev: "1", revisione: "1", da: valutazione.RevisioneDaCodice, revisioni: valutazione.RevisioniUguali},
		{nome: "il codice senza revisione, senza colonna", codice: "7120200A", rev: "", revisioni: nc, motivo: rnl},
		{nome: "il codice che non si legge: niente famiglia, la colonna non si legge", codice: "7120200X", rev: "1", revisioni: nc, motivo: rnl},
	} {
		c.insieme = r
		controllaColonna(t, c)
	}
}

// TestR113CodiceEColonnaDiscordi (R113 B: «non un'uguaglianza inventata e non un conflitto artificiale»): la revisione
// letta nel codice e quella letta nella colonna non concordano: non_confrontabili con revisione_vecchia_discorde, anche
// quando una delle due coincide con quella del file; il valore resta quello del codice, con la sua provenienza. Una
// colonna che la regola non legge non contraddice il codice. Due revisioni che la regola dichiara equivalenti non sono
// discordi: il confronto si fa con quella del codice (R-142 della revisione di B6b; R113: niente conflitti artificiali).
func TestR113CodiceEColonnaDiscordi(t *testing.T) {
	r := insiemeColonna(t, famigliaCatenaColonna(regolaColonnaACME("rev-colonna", "[0-9]")))
	nc, discorde := valutazione.RevisioniNonConfrontabili, valutazione.MotivoRevisioniVecchiaDiscorde
	for _, c := range []casoColonna{
		{nome: "il codice come il file, la colonna no", codice: "7120200A1", rev: "2", revisione: "1", da: valutazione.RevisioneDaCodice, revisioni: nc, motivo: discorde},
		{nome: "la colonna come il file, il codice no", codice: "7120200A2", rev: "1", revisione: "2", da: valutazione.RevisioneDaCodice, revisioni: nc, motivo: discorde},
		{nome: "la colonna che non si legge accanto al codice", codice: "7120200A1", rev: "B", revisione: "1", da: valutazione.RevisioneDaCodice, revisioni: valutazione.RevisioniUguali},
	} {
		c.insieme = r
		controllaColonna(t, c)
	}
	// la regola della colonna legge una o due cifre e dichiara equivalenti «1» e «01»: il codice legge «1», la colonna
	// «01», e le due non discordano (solo le equivalenze dichiarate: senza la dichiarazione sarebbero discordi)
	equivalente := regolaColonnaACME("rev-colonna", "[0-9]{1,2}")
	equivalente.Equivalenze = [][2]string{{"1", "01"}}
	controllaColonna(t, casoColonna{nome: "codice e colonna equivalenti per la regola", codice: "7120200A1", rev: "01", revisione: "1",
		da: valutazione.RevisioneDaCodice, revisioni: valutazione.RevisioniUguali, insieme: insiemeColonna(t, famigliaCatenaColonna(equivalente))})
	controllaColonna(t, casoColonna{nome: "codice e colonna senza l'equivalenza dichiarata", codice: "7120200A1", rev: "01", revisione: "1",
		da: valutazione.RevisioneDaCodice, revisioni: nc, motivo: discorde,
		insieme: insiemeColonna(t, famigliaCatenaColonna(regolaColonnaACME("rev-colonna", "[0-9]{1,2}")))})
}

// TestR113LaColonnaCheNonSiLeggePerStato (R113 B; T-B6-202): il codice letto senza revisione e la colonna non vuota che
// non si legge: un motivo per ogni stato della lettura, perché il banco conti la colonna per stato. Nessuna regola in
// campo separato: revisione_solo_in_colonna (R-65); una regola che non legge la colonna, una regola riservata, un token
// sospeso: revisione_colonna_non_interpretabile; due regole: revisione_colonna_ambigua. Mai una lettura per stringa.
func TestR113LaColonnaCheNonSiLeggePerStato(t *testing.T) {
	nc := valutazione.RevisioniNonConfrontabili
	riservata := regolaColonnaACME("rev-colonna", "[0-9]")
	riservata.Stato = grammatica.StatoRiservata
	token := regolaColonnaACME("rev-colonna", "[0-9]")
	token.TokenSospesi = []grammatica.TokenSospeso{{ID: "Q-ACME-2", Pattern: "x", Riserva: "Q-ACME-2"}}
	una := insiemeColonna(t, famigliaCatenaColonna(regolaColonnaACME("rev-colonna", "[0-9]")))
	for _, c := range []casoColonna{
		{nome: "nessuna regola in campo separato", codice: "7120200A", rev: "1", revisioni: nc, motivo: valutazione.MotivoRevisioniSoloInColonna,
			insieme: insiemeACME(t)},
		{nome: "la regola non legge la colonna per intero", codice: "7120200A", rev: "1A", revisioni: nc,
			motivo: valutazione.MotivoRevisioniColonnaNonInterpretabile, insieme: una},
		{nome: "una regola riservata", codice: "7120200A", rev: "1", revisioni: nc, motivo: valutazione.MotivoRevisioniColonnaNonInterpretabile,
			insieme: insiemeColonna(t, famigliaCatenaColonna(riservata))},
		{nome: "un token sospeso", codice: "7120200A", rev: "x", revisioni: nc, motivo: valutazione.MotivoRevisioniColonnaNonInterpretabile,
			insieme: insiemeColonna(t, famigliaCatenaColonna(token))},
		{nome: "due regole", codice: "7120200A", rev: "1", revisioni: nc, motivo: valutazione.MotivoRevisioniColonnaAmbigua,
			insieme: insiemeColonna(t, famigliaCatenaColonna(regolaColonnaACME("rev-colonna", "[0-9]"), regolaColonnaACME("rev-lettera", "[A-Z]")))},
	} {
		controllaColonna(t, c)
	}
}

// TestR113LaVersioneDellaLetturaNellEsito (E2 §2.6): la versione della lettura della colonna sta nell'esito, quindi nella
// sua impronta; la stessa fotografia con e senza la regola in campo separato dà due record piatti (la colonna letta, da
// «colonna», oppure nessuna provenienza) e due impronte.
func TestR113LaVersioneDellaLetturaNellEsito(t *testing.T) {
	th := scenaVecchio(t, "7120200A", `{}`)
	senza := calcola(t, fotografiaDi(th), insiemeACME(t), valutazione.Ingressi{})
	con := calcola(t, fotografiaDi(th), insiemeColonna(t, famigliaCatenaColonna(regolaColonnaACME("rev-colonna", "[0-9]"))), valutazione.Ingressi{})
	if senza.VersioneRevisioneRegistrata != motorea.VersioneRevisioneRegistrata || con.VersioneRevisioneRegistrata != "revisione-registrata-1" {
		t.Errorf("versioni %q e %q", senza.VersioneRevisioneRegistrata, con.VersioneRevisioneRegistrata)
	}
	vs := confrontabileDi(t, esitoThread(t, senza, threadACME), aDisegno).Vecchio
	vc := confrontabileDi(t, esitoThread(t, con, threadACME), aDisegno).Vecchio
	if vs.RevisioneDa != "" || vc.RevisioneDa != valutazione.RevisioneDaColonna || senza.Impronta == con.Impronta {
		t.Errorf("provenienze %q e %q, impronte %s e %s", vs.RevisioneDa, vc.RevisioneDa, senza.Impronta, con.Impronta)
	}
}
