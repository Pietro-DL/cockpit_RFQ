package motorea

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// GP-1 (addendum «contesto partner», 16/10): il contesto di lavorazione, i partner, i legami, i ruoli. Tutto è
// inventato: ACME è il cliente, BETA il fornitore che disegna per lui; i codici imitano solo la FORMA dei casi della
// direttiva 21 §9 (un codice a punti, un 7 cifre ambiguo, un 6 cifre del fornitore, un 7 cifre compatto del
// fornitore), mai i valori veri.

const shaP1 = "1111111111111111111111111111111111111111111111111111111111111111"

// grammaticaBETA: l'emittente partner, con il 6 cifre del disegno e un 7 cifre compatto che comincia per 12.
func grammaticaBETA() grammatica.Grammatica {
	return grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Emittente:      &grammatica.EmittenteGrammatica{ID: "beta", Nome: "BETA (inventato)"},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloCompleto},
		Famiglie: []grammatica.FamigliaCodice{
			{
				ID: "beta-dis", Namespace: "beta-dis", Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
				Base:   base(segPattern("disegno", "[0-9]{6}", "")),
				Forme:  []grammatica.FormaCodice{forma("ovunque", sel("nome_file", "nodo_step.id", "radice_step.id", "cartiglio.codice"), pBase())},
				Esempi: []grammatica.EsempioCodice{positivo("e-nome", "nome_file", "654321.pdf", grammatica.LetturaAttesa{Forma: "ovunque", Base: "654321"})},
			},
			{
				ID: "beta-compatto", Namespace: "beta-compatto", Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
				Base:   base(segPattern("numero", "128[0-9]{4}", "")),
				Forme:  []grammatica.FormaCodice{forma("nodo", sel("nodo_step.id"), pBase())},
				Esempi: []grammatica.EsempioCodice{positivo("e-nodo", "nodo_step.id", "1287654", grammatica.LetturaAttesa{Forma: "nodo", Base: "1287654"})},
			},
		},
	}
}

// famigliePartnerACME: le famiglie del cliente per le prove GP-1.
func famigliePartnerACME() []grammatica.FamigliaCodice {
	punti := grammatica.FamigliaCodice{
		ID: "acme-punti", Namespace: "acme-punti", Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("g", "[0-9]{2}", ""), segPattern("s", "[0-9]{4}", "."), segPattern("n", "[0-9]{4}", ".")),
		Forme: []grammatica.FormaCodice{forma("ovunque", sel("nome_file", "radice_step.id", "nodo_step.id", "corpo"), pBase())},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-punti", "corpo", "91.8888.7777", grammatica.LetturaAttesa{Forma: "ovunque", Base: "91.8888.7777"}),
		},
	}
	punti.Forme[0].SuffissiIgnoti = grammatica.SuffissiIgnotiSegnala
	num := grammatica.FamigliaCodice{
		ID: "acme-num", Namespace: "acme-num", Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base:  base(segPattern("n", "1[23][0-9]{5}", "")),
		Forme: []grammatica.FormaCodice{forma("contestuale", sel("nodo_step.id", "nome_file"), pBase())},
	}
	num.Forme[0].Stato = grammatica.StatoContestuale
	e := positivo("e-num", "nodo_step.id", "1309991", grammatica.LetturaAttesa{Forma: "contestuale", Base: "1309991"})
	e.Attivazione = &grammatica.AttivazioneEsempio{Codifica: grammatica.CodificaCliente}
	num.Esempi = []grammatica.EsempioCodice{e}
	otto := grammatica.FamigliaCodice{
		ID: "acme-otto", Namespace: "acme-otto", Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("n", "[0-9]{8}", "")),
		Forme: []grammatica.FormaCodice{forma("con-s", sel("corpo"), pBase(), grammatica.Parte{Tipo: grammatica.TipoParteMarcatore, Pattern: "S[0-9]{4}", Min: 0, Max: 1})},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-s", "corpo", "58880000S1234", grammatica.LetturaAttesa{Forma: "con-s", Base: "58880000", Marcatore: "S1234"}),
			positivo("e-nudo", "corpo", "58880000", grammatica.LetturaAttesa{Forma: "con-s", Base: "58880000"}),
		},
	}
	mail := grammatica.FamigliaCodice{
		ID: "acme-mail", Namespace: "acme-mail", Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto},
		Base:   base(segLetterale("k", "K", ""), segPattern("n", "[0-9]{6}", "")),
		Forme:  []grammatica.FormaCodice{forma("menzione", sel("corpo"), pBase())},
		Esempi: []grammatica.EsempioCodice{positivo("e-mail", "corpo", "K123456", grammatica.LetturaAttesa{Forma: "menzione", Base: "K123456"})},
	}
	mail.Forme[0].FunzioneMassima = "menzione"
	return []grammatica.FamigliaCodice{punti, num, otto, mail}
}

func pezzoL(ruolo string, famiglie ...string) grammatica.PezzoLegame {
	return grammatica.PezzoLegame{Tipo: grammatica.PezzoLettura, Ruolo: ruolo, Famiglie: famiglie}
}

func pezzoS(letterali ...string) grammatica.PezzoLegame {
	return grammatica.PezzoLegame{Tipo: grammatica.PezzoSeparatore, Letterali: letterali}
}

func pezzoE(letterali ...string) grammatica.PezzoLegame {
	return grammatica.PezzoLegame{Tipo: grammatica.PezzoEstensione, Letterali: letterali}
}

func esempioLegame(id, s, testo string, modello string, letture ...grammatica.LetturaAttesa) grammatica.EsempioLegame {
	return grammatica.EsempioLegame{ID: id, Origine: grammatica.OrigineSintetico, Selettore: s, Testo: testo,
		Attivazione: grammatica.AttivazioneEsempio{Partner: []string{"beta"}, Codifica: grammatica.CodificaMista, Modelli: []string{modello}},
		Atteso:      grammatica.AttesoLegame{Letture: letture}}
}

func legamiACME() []grammatica.ModelloLegame {
	return []grammatica.ModelloLegame{
		{ID: "assieme", Stato: grammatica.StatoAttiva, Selettori: sel("nome_file"),
			Schema: []grammatica.PezzoLegame{pezzoL(grammatica.RuoloLegameCliente, "acme-punti"), pezzoS("__"), pezzoL(grammatica.RuoloLegameFornitore, "beta-dis"), pezzoS("_ASM"), pezzoE("STEP", "stp")},
			Esempi: []grammatica.EsempioLegame{esempioLegame("e1", "nome_file", "91.8888.7777__654301_ASM.STEP", "assieme",
				grammatica.LetturaAttesa{Ruolo: "primaria", Famiglia: "acme-punti", Base: "91.8888.7777"},
				grammatica.LetturaAttesa{Ruolo: "riferimento", Famiglia: "beta-dis", Base: "654301"})}},
		{ID: "assieme-radice", Stato: grammatica.StatoAttiva, Selettori: sel("radice_step.id"),
			Schema: []grammatica.PezzoLegame{pezzoL(grammatica.RuoloLegameCliente, "acme-punti"), pezzoS("__"), pezzoL(grammatica.RuoloLegameFornitore, "beta-dis"), pezzoS("_ASM")},
			Esempi: []grammatica.EsempioLegame{esempioLegame("e1", "radice_step.id", "91.8888.7777__654301_ASM", "assieme-radice",
				grammatica.LetturaAttesa{Ruolo: "primaria", Famiglia: "acme-punti"}, grammatica.LetturaAttesa{Ruolo: "riferimento", Famiglia: "beta-dis"})}},
		{ID: "disegno", Stato: grammatica.StatoAttiva, Selettori: sel("nome_file"),
			Schema: []grammatica.PezzoLegame{pezzoL(grammatica.RuoloLegameCliente, "acme-num"), pezzoS("("), pezzoL(grammatica.RuoloLegameFornitore, "beta-dis"), pezzoS(")"), pezzoE("pdf")},
			Esempi: []grammatica.EsempioLegame{esempioLegame("e1", "nome_file", "1309991(765432).pdf", "disegno",
				grammatica.LetturaAttesa{Ruolo: "primaria", Famiglia: "acme-num", Base: "1309991"}, grammatica.LetturaAttesa{Ruolo: "riferimento", Famiglia: "beta-dis", Base: "765432"})}},
		{ID: "varianti", Stato: grammatica.StatoAttiva, Selettori: sel("nome_file"),
			Schema: []grammatica.PezzoLegame{pezzoL(grammatica.RuoloLegameVariante, "beta-dis"), pezzoS("_"), pezzoL(grammatica.RuoloLegameVariante, "beta-dis"), pezzoE("pdf")},
			Esempi: []grammatica.EsempioLegame{
				esempioLegame("e1", "nome_file", "765400_765401.pdf", "varianti",
					grammatica.LetturaAttesa{Ruolo: "variante", Base: "765400"}, grammatica.LetturaAttesa{Ruolo: "variante", Base: "765401"}),
				{ID: "e2", Origine: grammatica.OrigineSintetico, Selettore: "nome_file", Testo: "765400_765401X.pdf",
					Attivazione: grammatica.AttivazioneEsempio{Partner: []string{"beta"}, Modelli: []string{"varianti"}}, Atteso: grammatica.AttesoLegame{Nessuno: true}},
			}},
	}
}

// motorePartner: il motore di ACME con BETA dichiarato e caricato (hash giusto), o con l'hash dato se non vuoto.
func motorePartner(t *testing.T, hash string) (*Motore, string, []evidenze.Diagnostica) {
	t.Helper()
	lim := limitiACME()
	sb, err := grammatica.NuovoSnapshot(fileJSON(t, grammaticaBETA()), lim)
	if err != nil {
		t.Fatalf("BETA: %v\n%s", err, elenco(diagnosticheDi(err)))
	}
	if _, d, err := CompilaVerificato(sb, lim); err != nil {
		t.Fatalf("BETA da solo: %v\n%s", err, elenco(d))
	}
	if hash == "" {
		hash = sb.Hash
	}
	g := grammaticaACME(famigliePartnerACME()...)
	g.Partner = []grammatica.DichiarazionePartner{{Emittente: "beta", HashSnapshot: hash, Selettori: sel("nome_file", "nodo_step.id", "radice_step.id", "cartiglio.codice")}}
	g.Legami = legamiACME()
	sa, err := grammatica.NuovoSnapshot(fileJSON(t, g), lim)
	if err != nil {
		t.Fatalf("ACME: %v\n%s", err, elenco(diagnosticheDi(err)))
	}
	m, d, err := CompilaVerificatoConPartner(sa, lim, map[string]PartnerCaricato{"beta": {Snapshot: sb}})
	if err != nil {
		t.Fatalf("ACME con BETA: %v\n%s", err, elenco(d))
	}
	return m, sb.Hash, d
}

// docAllegato: un documento di un allegato con il nome ricevuto e lo sha dati, e le unità.
func docAllegato(t *testing.T, nome, sha string, entita []evidenze.EntitaLocale, unita ...evidenze.UnitaEvidenza) evidenze.DocumentoEvidenze {
	t.Helper()
	id := uuid.MustParse("00000000-0000-4000-8000-0000000000f1")
	fid := "allegato:" + id.String()
	for i := range unita {
		unita[i].FonteID = fid
	}
	return evidenze.DocumentoEvidenze{
		BundleID: "bundle-gp1-" + nome, VersioneSchema: evidenze.VersioneSchemaDocumento, VersioneAdattatore: "adattatori-dsint",
		Fonte: evidenze.Fonte{ID: fid, Tipo: "allegato", OrigineID: id,
			Provenienza:      evidenze.Provenienza{NomeRicevuto: nome},
			RiferimentoFatti: evidenze.RiferimentoFatti{Tipo: "analisi_file", Sha256: sha, VersioneAnalizzatore: 1, HashConfigurazione: "h"}},
		Testi:   []evidenze.TestoOriginale{{ID: "allegato.nome_file", Testo: nome, Origine: "allegato.nome_file"}},
		Entita:  entita,
		Unita:   unita,
		Qualita: evidenze.QualitaFonte{Stato: "disponibile"},
	}
}

func uNome(t *testing.T, nome string) evidenze.UnitaEvidenza {
	return evidenze.UnitaEvidenza{ID: "u:nome", Selettore: selettore(t, "nome_file"), Testo: nome,
		Posizione: evidenze.Localizzatore{Tipo: "nome_file", NomeFile: &evidenze.PosNomeFile{Campo: "allegato.nome_file", Intervallo: evidenze.Intervallo{Inizio: 0, Fine: len(nome)}}},
		Qualita:   evidenze.QualitaUnita{Localizzazione: "esatta", Metodo: "adattatore"}}
}

// contesto: un contesto dichiarato per ACME con BETA attivo, il perimetro e le codifiche dati.
func contesto(hash string, perimetro []OccorrenzaContesto, codifiche []CodificaContesto, modelli ...string) ContestoLavorazione {
	return ContestoLavorazione{VersioneContesto: VersioneContesto, ID: "ctx-acme-1", RFQ: "rfq-acme-1", ClienteID: clienteACME,
		Stato: StatoContestoDichiarato, Provenienza: ProvenienzaContesto{Autore: "banco", Data: "2026-10-16"},
		Partner: []PartnerContesto{{Emittente: "beta", HashSnapshot: hash}}, Perimetro: perimetro, Codifiche: codifiche, Modelli: modelli}
}

func risolvi(t *testing.T, m *Motore, c ContestoLavorazione) *Motore {
	t.Helper()
	r, _, err := RisolviContesto(m, c, nil)
	if err != nil {
		t.Fatalf("contesto: %v\n%s", err, elenco(diagnosticheDi(err)))
	}
	return m.ConContesto(r)
}

func lettureDi(r Interpretazione, famiglia string) []LetturaCodice {
	var out []LetturaCodice
	for _, l := range r.Letture {
		if l.Forma.Famiglia == famiglia {
			out = append(out, l)
		}
	}
	return out
}

// Partnership assente: nessuna lettura del partner e nessuna forma contestuale (direttiva 21 §9, prima riga).
func TestGP1SenzaContestoNessunPartner(t *testing.T) {
	m, _, _ := motorePartner(t, "")
	r := interpreta(t, m, docAllegato(t, "654321.pdf", shaP1, nil, uNome(t, "654321.pdf")), evidenze.UsoSconosciuto("bundle-gp1-654321.pdf"))
	if len(r.Letture) != 0 || r.ContestoID != "" {
		t.Errorf("senza contesto nessuna lettura di BETA: %+v", r.Letture)
	}
	r = interpreta(t, m, docAllegato(t, "1309991.pdf", shaP1, nil, uNome(t, "1309991.pdf")), evidenze.UsoSconosciuto("bundle-gp1-1309991.pdf"))
	if len(r.Letture) != 0 {
		t.Errorf("senza contesto il 7 cifre contestuale non legge: %+v", r.Letture)
	}
}

// 0766: radice del cliente con il riferimento del fornitore nel nome, nessuna discordanza; la relazione è proposta.
func TestGP1LegameAssiemeConRiferimento(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	nome := "91.8888.7777__654301_ASM.STEP"
	mc := risolvi(t, m, contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: nome}},
		[]CodificaContesto{{Selettore: "nome_file", Emittente: grammatica.CodificaMista}}, "assieme"))
	r := interpreta(t, mc, docAllegato(t, nome, shaP1, nil, uNome(t, nome)), evidenze.UsoSconosciuto("bundle-gp1-"+nome))
	a, b := lettureDi(r, "acme-punti"), lettureDi(r, "beta-dis")
	if len(a) != 1 || a[0].Ruolo != "" || len(b) != 1 || b[0].Ruolo != grammatica.RuoloLetturaRiferimento {
		t.Fatalf("primaria del cliente e riferimento del fornitore: %+v / %+v", a, b)
	}
	if b[0].Forma.Emittente != "beta" || b[0].Forma.HashEmittente != hash {
		t.Errorf("la provenienza del riferimento: %+v", b[0].Forma)
	}
	if len(r.Relazioni) != 1 || r.Relazioni[0].Tipo != RelazioneDocumentatoDa || r.Relazioni[0].Stato != StatoRelazioneProposta ||
		r.Relazioni[0].LetturaSorgente != a[0].ID || len(r.Relazioni[0].LettureBersaglio) != 1 || r.Relazioni[0].LettureBersaglio[0] != b[0].ID {
		t.Errorf("la relazione documentato_da proposta: %+v", r.Relazioni)
	}
	if r.ContestoID != "ctx-acme-1" || r.ImprontaContesto == "" {
		t.Errorf("il contesto nell'identità: %q %q", r.ContestoID, r.ImprontaContesto)
	}
}

// Lo stesso sha con un altro nome, o un altro sha con lo stesso nome: fuori perimetro, nessuna eredità.
func TestGP1PerimetroPerOccorrenza(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	mc := risolvi(t, m, contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "654321.pdf"}}, nil))
	for _, c := range []struct{ nome, sha string }{{"654322.pdf", shaP1}, {"654321.pdf", strings.Repeat("2", 64)}} {
		r := interpreta(t, mc, docAllegato(t, c.nome, c.sha, nil, uNome(t, c.nome)), evidenze.UsoSconosciuto("bundle-gp1-"+c.nome))
		if len(r.Letture) != 0 || r.ContestoID != "" {
			t.Errorf("%s/%s fuori perimetro: %+v", c.nome, c.sha[:4], r.Letture)
		}
	}
	r := interpreta(t, mc, docAllegato(t, "654321.pdf", shaP1, nil, uNome(t, "654321.pdf")), evidenze.UsoSconosciuto("bundle-gp1-654321.pdf"))
	if len(lettureDi(r, "beta-dis")) != 1 {
		t.Errorf("nel perimetro BETA legge: %+v", r.Letture)
	}
}

// 0675: il 7 cifre del cliente sui nodi solo con la codifica del campo; il nome <7>(<6>) dà la relazione.
func TestGP1ContestoNumericoEDisegno(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	step := docAllegato(t, "porta.stp", shaP1, []evidenze.EntitaLocale{entNodo("e:step:#1", "#1")},
		uStep(t, "u:step:#1:id", "e:step:#1", "#1", "nodo_step.id", "id", "1309991"))
	mc := risolvi(t, m, contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "porta.stp"}, {Sha256: shaP1, Nome: "1309991(765432).pdf"}},
		[]CodificaContesto{{Selettore: "nodo_step.id", Emittente: grammatica.CodificaCliente, Perimetro: []int{0}}}, "disegno"))
	r := interpreta(t, mc, step, evidenze.UsoSconosciuto(step.BundleID))
	if n := lettureDi(r, "acme-num"); len(n) != 1 || n[0].Ruolo != "" {
		t.Errorf("il 7 cifre del cliente sul nodo: %+v", r.Letture)
	}
	nome := "1309991(765432).pdf"
	r = interpreta(t, mc, docAllegato(t, nome, shaP1, nil, uNome(t, nome)), evidenze.UsoSconosciuto("bundle-gp1-"+nome))
	if b := lettureDi(r, "beta-dis"); len(b) != 1 || b[0].Ruolo != grammatica.RuoloLetturaRiferimento || len(r.Relazioni) != 1 {
		t.Errorf("il disegno del fornitore come riferimento dell'articolo: %+v / %+v", r.Letture, r.Relazioni)
	}
}

// Campo non dichiarato con il partner attivo: le due letture dello stesso letterale restano alternative.
func TestGP1CampoNonDichiaratoAlternative(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	step := docAllegato(t, "porta.stp", shaP1, []evidenze.EntitaLocale{entNodo("e:step:#1", "#1")},
		uStep(t, "u:step:#1:id", "e:step:#1", "#1", "nodo_step.id", "id", "1287654"))
	mc := risolvi(t, m, contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "porta.stp"}}, nil))
	r := interpreta(t, mc, step, evidenze.UsoSconosciuto(step.BundleID))
	if len(lettureDi(r, "acme-num")) != 1 || len(lettureDi(r, "beta-compatto")) != 1 || len(conCodice(r.Diagnostiche, CodiceMotoreLettureAlternative)) == 0 {
		t.Errorf("le alternative restano visibili, nessuna scelta: %+v\n%s", r.Letture, elenco(r.Diagnostiche))
	}
	// Con il campo dichiarato del fornitore il 7 cifre del cliente tace, ma in ombra si vede.
	mc = risolvi(t, m, contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "porta.stp"}},
		[]CodificaContesto{{Selettore: "nodo_step.id", Emittente: "beta"}}))
	r = interpreta(t, mc, step, evidenze.UsoSconosciuto(step.BundleID))
	if len(lettureDi(r, "acme-num")) != 0 || len(lettureDi(r, "beta-compatto")) != 1 {
		t.Errorf("campo dichiarato del fornitore: %+v", r.Letture)
	}
	if len(conCodice(r.Diagnostiche, CodiceContestoCodificaContraddetta)) != 1 {
		t.Errorf("la lettura del cliente in ombra resta visibile come diagnostica:\n%s", elenco(r.Diagnostiche))
	}
}

// Due varianti nella stessa tavola: nessuna scelta del primo numero; il nome quasi giusto non dà relazioni.
func TestGP1Varianti(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	for _, c := range []struct {
		nome      string
		relazioni int
	}{{"765400_765401.pdf", 1}, {"765400_765401X.pdf", 0}} {
		mc := risolvi(t, m, contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: c.nome}}, nil, "varianti"))
		r := interpreta(t, mc, docAllegato(t, c.nome, shaP1, nil, uNome(t, c.nome)), evidenze.UsoSconosciuto("bundle-gp1-"+c.nome))
		if len(r.Relazioni) != c.relazioni {
			t.Errorf("%s: relazioni %+v", c.nome, r.Relazioni)
		}
		varianti := 0
		for _, l := range r.Letture {
			if l.Ruolo == grammatica.RuoloLetturaVariante {
				varianti++
			}
		}
		if (c.relazioni == 1 && varianti != 2) || (c.relazioni == 0 && varianti != 0) {
			t.Errorf("%s: varianti %d: %+v", c.nome, varianti, r.Letture)
		}
	}
}

// Un partner dichiarato dal contesto con un hash che il motore non ha: mai un esito completo.
func TestGP1PartnerNonCaricabile(t *testing.T) {
	m, _, _ := motorePartner(t, "")
	c := contesto(strings.Repeat("9", 64), []OccorrenzaContesto{{Sha256: shaP1, Nome: "654321.pdf"}}, nil)
	r0, diag, err := RisolviContesto(m, c, nil)
	if err != nil || len(conCodice(diag, CodiceContestoPartnerNonCaricabile)) != 1 {
		t.Fatalf("il contesto si applica, con l'avviso: %v\n%s", err, elenco(diag))
	}
	r := interpreta(t, m.ConContesto(r0), docAllegato(t, "654321.pdf", shaP1, nil, uNome(t, "654321.pdf")), evidenze.UsoSconosciuto("bundle-gp1-654321.pdf"))
	if r.Stato != StatoInterpretazioneParziale || len(lettureDi(r, "beta-dis")) != 0 {
		t.Errorf("parziale e senza letture del partner: %s %+v", r.Stato, r.Letture)
	}
}

// Un partner in deriva (hash bloccato dal cliente diverso da quello caricato) non entra; il cliente resta attivo.
func TestGP1PartnerInDeriva(t *testing.T) {
	m, _, d := motorePartner(t, strings.Repeat("8", 64))
	if m == nil || len(conCodice(d, CodicePartnerNonComposto)) != 1 || len(m.PartnerAmmessi()) != 0 {
		t.Errorf("il cliente resta attivo senza il partner in deriva:\n%s", elenco(d))
	}
}

// L'impronta del contesto non dipende dall'ordine del perimetro e delle codifiche.
func TestGP1ImprontaContestoDeterministica(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	a := contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "a.pdf"}, {Sha256: shaP1, Nome: "b.pdf"}},
		[]CodificaContesto{{Selettore: "nome_file", Emittente: "beta", Perimetro: []int{1}}, {Selettore: "nodo_step.id", Emittente: grammatica.CodificaCliente}})
	b := contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "b.pdf"}, {Sha256: shaP1, Nome: "a.pdf"}},
		[]CodificaContesto{{Selettore: "nodo_step.id", Emittente: grammatica.CodificaCliente}, {Selettore: "nome_file", Emittente: "beta", Perimetro: []int{0}}})
	ra, _, err1 := RisolviContesto(m, a, nil)
	rb, _, err2 := RisolviContesto(m, b, nil)
	if err1 != nil || err2 != nil || ra.Impronta != rb.Impronta {
		t.Errorf("impronte %v %v: %s / %s", err1, err2, ra.Impronta, rb.Impronta)
	}
}

// T-E1-07 resta: due primarie incompatibili sullo stesso nodo restano due letture (il controllo sta in
// ancoraggio); qui: un riferimento di un legame non è mai una primaria.
func TestGP1RiferimentoMaiPrimaria(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	nome := "91.8888.7777__654301_ASM.STEP"
	mc := risolvi(t, m, contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: nome}}, []CodificaContesto{{Selettore: "nome_file", Emittente: grammatica.CodificaCliente}}, "assieme"))
	r := interpreta(t, mc, docAllegato(t, nome, shaP1, nil, uNome(t, nome)), evidenze.UsoSconosciuto("bundle-gp1-"+nome))
	// Con il nome codificato dal cliente il fornitore non legge libero, ma dentro il legame sì, come riferimento.
	for _, l := range lettureDi(r, "beta-dis") {
		if l.Ruolo != grammatica.RuoloLetturaRiferimento {
			t.Errorf("una lettura del fornitore nel nome del cliente senza ruolo riferimento: %+v", l)
		}
	}
	if len(lettureDi(r, "beta-dis")) != 1 {
		t.Errorf("il riferimento nel legame: %+v", r.Letture)
	}
}

// G-7, G-5 e il marcatore a pattern (PM-38): funzione massima, suffisso non letto, S e quattro cifre.
func TestGP1FunzioneSuffissoMarcatore(t *testing.T) {
	m, _, _ := motorePartner(t, "")
	corpo := "K123456 91.8888.7777-PROTO 58880000S1234"
	doc := docDSint([]evidenze.TestoOriginale{testoOrig(corpoDSint, corpo)}, []evidenze.Segmento{segDSint("s:corrente", "corrente", "m0", corpoDSint, 0, len(corpo))}, nil,
		[]evidenze.UnitaEvidenza{uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo, 0)})
	r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
	for _, l := range r.Letture {
		switch l.Forma.Famiglia {
		case "acme-mail":
			if l.Funzione != FunzMenzione {
				t.Errorf("funzione massima menzione: %s", l.Funzione)
			}
		case "acme-punti":
			if l.Qualita != QualitaDaVerificare {
				t.Errorf("il suffisso non letto toglie la completezza: %+v", l)
			}
		case "acme-otto":
			if l.Forma.Marcatore == nil || l.Forma.Marcatore.Valore != "S1234" || l.Forma.Base.Normalizzata != "58880000" {
				t.Errorf("S e quattro cifre come marcatore: %+v", l.Forma)
			}
		}
	}
	if len(conCodice(r.Diagnostiche, CodiceMotoreSuffissoNonLetto)) != 1 {
		t.Errorf("la diagnostica del suffisso:\n%s", elenco(r.Diagnostiche))
	}
}

// La grammatica di un emittente non legge mai sui campi della mail.
func TestGP1EmittenteSenzaMail(t *testing.T) {
	g := grammaticaBETA()
	g.Famiglie[0].Forme[0].Selettori = append(g.Famiglie[0].Forme[0].Selettori, "corpo")
	if _, err := grammatica.NuovoSnapshot(fileJSON(t, g), limitiACME()); err == nil {
		t.Error("un emittente con una forma sul corpo deve essere rifiutato")
	}
}
