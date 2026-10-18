// L1 — le regole di lettura di un thread per il registro d'importazione (V1, piano della 0023 §14 passo 6; emendamento
// di V1 §3.8 «regole» e §3.8.2; RM-13): RegoleDiLettura dà, con le stesse funzioni di Calcola, l'impronta dell'indice,
// la grammatica del cliente, il contesto applicato con i partner attivi e la copia canonica, e il perimetro per
// occorrenza con lo stesso criterio dell'attivazione. La prova confronta ogni esposizione con l'esito di Calcola sulla
// stessa fotografia: il perimetro esposto è esattamente l'insieme dei file la cui interpretazione porta il contesto.
package valutazione_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I clienti di queste prove sono inventati, come in scena_test.go: ACME il cliente, BETA il fornitore che disegna per lui
// (un emittente partner con un codice di disegno di sei cifre).

func shaDi(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// insiemeConPartner: ACME, con la famiglia della catena e il partner BETA dichiarato con l'hash della sua grammatica, e
// l'emittente BETA nell'indice, tutti e due dalla porta del prodotto (CompilaInsieme); poi i contesti dati. Dà anche
// l'hash della grammatica di BETA.
func insiemeConPartner(t *testing.T, contesti ...motorea.ContestoLavorazione) (*motorea.InsiemeRegole, string) {
	t.Helper()
	lim := limitiACME()
	selPartner := []string{"nome_file", "nodo_step.id", "radice_step.id", "cartiglio.codice"}
	beta := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Emittente:      &grammatica.EmittenteGrammatica{ID: "beta", Nome: "BETA (inventato)"},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloCompleto},
		Famiglie: []grammatica.FamigliaCodice{{
			ID: "beta-dis", Namespace: "beta-dis", Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
			Base: grammatica.Base{Segmenti: []grammatica.SegmentoBase{{Nome: "disegno", Pattern: "[0-9]{6}", Identitario: true}},
				Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
				ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII},
			Forme: []grammatica.FormaCodice{{ID: "ovunque", Selettori: selPartner, Stato: grammatica.StatoAttiva, Completa: true,
				Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
			Esempi: []grammatica.EsempioCodice{{ID: "e-nome", Origine: grammatica.OrigineSintetico, Selettore: "nome_file", Testo: "654321.pdf",
				Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "ovunque", Base: "654321"}}}}},
		}},
	}
	rawB, err := json.Marshal(beta)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := grammatica.NuovoSnapshot(rawB, lim)
	if err != nil {
		t.Fatalf("BETA: %v", err)
	}
	acme := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: clienteACME, RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       []grammatica.FamigliaCodice{famigliaCatena()},
		Partner:        []grammatica.DichiarazionePartner{{Emittente: "beta", HashSnapshot: sb.Hash, Selettori: selPartner}},
	}
	rawA, err := json.Marshal(acme)
	if err != nil {
		t.Fatal(err)
	}
	ind := grammatica.IndiceRegole{VersioneIndice: grammatica.VersioneIndice, Limiti: lim,
		Grammatiche: []grammatica.VoceIndice{{ClienteID: clienteACME, File: "acme.v1.json", Sha256: shaDi(rawA)}},
		Emittenti:   []grammatica.VoceEmittente{{ID: "beta", File: "beta.v1.json", Sha256: shaDi(rawB)}}}
	r, d, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": rawA, "beta.v1.json": rawB})
	if err != nil || r.Motori[clienteACME] == nil || r.Emittenti["beta"] == nil || len(r.Motori[clienteACME].PartnerAmmessi()) != 1 {
		t.Fatalf("insieme con il partner: %v, motori %d, emittenti %d\n%+v", err, len(r.Motori), len(r.Emittenti), d)
	}
	r.Contesti = contesti
	return &r, sb.Hash
}

// contestoACME: un contesto dichiarato per il thread, con BETA attivo e il perimetro dato.
func contestoACME(thread uuid.UUID, hash string, perimetro ...motorea.OccorrenzaContesto) motorea.ContestoLavorazione {
	return motorea.ContestoLavorazione{VersioneContesto: motorea.VersioneContesto, ID: "ctx-acme-v1", RFQ: thread.String(), ClienteID: clienteACME,
		Stato: motorea.StatoContestoDichiarato, Provenienza: motorea.ProvenienzaContesto{Autore: "qa", Data: "2026-10-18"},
		Partner: []motorea.PartnerContesto{{Emittente: "beta", HashSnapshot: hash}}, Perimetro: perimetro}
}

// controllaPerimetro: per ogni file dell'esito del thread il perimetro esposto è l'attivazione: NelPerimetro se e solo se
// l'interpretazione del file porta il contesto, e allora con l'ID e l'impronta del contesto esposto. Dà i file nel
// perimetro, in ordine.
func controllaPerimetro(t *testing.T, nome string, reg valutazione.RegoleDelThread, et valutazione.EsitoThread) []uuid.UUID {
	t.Helper()
	var dentro []uuid.UUID
	for _, f := range et.File {
		letto := f.Interpretazione.ContestoID != ""
		if reg.NelPerimetro(f) != letto {
			t.Errorf("%s: il file %s è nel perimetro esposto %v, ma l'interpretazione ha il contesto %q", nome, f.AllegatoID, reg.NelPerimetro(f), f.Interpretazione.ContestoID)
		}
		if !letto {
			continue
		}
		dentro = append(dentro, f.AllegatoID)
		if reg.Contesto == nil || f.Interpretazione.ContestoID != reg.Contesto.ID || f.Interpretazione.ImprontaContesto != reg.Contesto.Impronta {
			t.Errorf("%s: il file %s ha il contesto %q %q, quello esposto è %+v", nome, f.AllegatoID, f.Interpretazione.ContestoID,
				f.Interpretazione.ImprontaContesto, reg.Contesto)
		}
	}
	sort.Slice(dentro, func(i, j int) bool { return dentro[i].String() < dentro[j].String() })
	return dentro
}

// sottoinsieme: ogni diagnostica di a sta anche in b.
func sottoinsieme(a, b []evidenze.Diagnostica) bool {
	for _, x := range a {
		trovata := false
		for _, y := range b {
			trovata = trovata || reflect.DeepEqual(x, y)
		}
		if !trovata {
			return false
		}
	}
	return true
}

// TestRegoleDiLetturaComeCalcola: con un contesto dichiarato per il thread (lo STEP per allegato, la scansione per sha256
// e nome), le regole esposte sono quelle dell'esito: l'impronta dell'indice e la versione dei limiti, il cliente e la
// grammatica; il contesto è attivo, con BETA fra i partner attivi e l'hash della sua grammatica, la copia canonica ha
// l'impronta del contesto, e il perimetro esposto è esattamente l'insieme dei file letti con il contesto. Le
// diagnostiche esposte sono fra quelle del thread. Due chiamate danno le stesse regole.
func TestRegoleDiLetturaComeCalcola(t *testing.T) {
	th := scenaCollegamento(t)
	_, hash := insiemeConPartner(t)
	r, _ := insiemeConPartner(t, contestoACME(th.ID, hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()},
		motorea.OccorrenzaContesto{Sha256: sha("c"), Nome: "scansione-acme.pdf"}))
	f := fotografia(th)
	e := calcola(t, f, r, valutazione.Ingressi{})
	et := e.Thread[0]
	reg, ok := valutazione.RegoleDiLettura(f, r, th.ID)
	if !ok {
		t.Fatal("il thread della fotografia non c'è")
	}
	if reg.ImprontaIndice == "" || reg.ImprontaIndice != e.ImprontaIndice || reg.VersioneLimiti != e.VersioneLimiti {
		t.Errorf("la testata delle regole %q %q, l'esito %q %q", reg.ImprontaIndice, reg.VersioneLimiti, e.ImprontaIndice, e.VersioneLimiti)
	}
	if reg.ClienteID != et.ClienteID || reg.HashGrammatica == "" || reg.HashGrammatica != et.HashSnapshot {
		t.Errorf("la grammatica del cliente %s %q, l'esito %s %q", reg.ClienteID, reg.HashGrammatica, et.ClienteID, et.HashSnapshot)
	}
	c := reg.Contesto
	if !c.Attivo() || c.ID != "ctx-acme-v1" || c.Impronta == "" {
		t.Fatalf("il contesto applicato: %+v", c)
	}
	if got, want := c.PartnerAttivi(), []motorea.PartnerAttivo{{Emittente: "beta", Hash: hash}}; !reflect.DeepEqual(got, want) {
		t.Errorf("partner attivi %+v, attesi %+v", got, want)
	}
	copia := c.CopiaCanonica()
	if imp, err := jsoncanonico.ImprontaDi(copia); err != nil || imp != c.Impronta || copia.Padre != nil || copia.Contesto.RFQ != th.ID.String() {
		t.Errorf("la copia canonica: impronta %s (%v), quella del contesto %s; %+v", imp, err, c.Impronta, copia)
	}
	dentro := controllaPerimetro(t, "con il contesto", reg, et)
	want := []uuid.UUID{aStepB, aScansione}
	sort.Slice(want, func(i, j int) bool { return want[i].String() < want[j].String() })
	if !reflect.DeepEqual(dentro, want) {
		t.Errorf("file nel perimetro %v, attesi lo STEP e la scansione %v", dentro, want)
	}
	if len(et.File) <= len(dentro) {
		t.Errorf("tutti i %d file nel perimetro: la prova vuole anche file fuori", len(et.File))
	}
	if !sottoinsieme(reg.Diagnostiche, et.Diagnostiche) {
		t.Errorf("diagnostiche esposte fuori dall'esito: %+v", reg.Diagnostiche)
	}
	di, _ := valutazione.RegoleDiLettura(f, r, th.ID)
	if di.ImprontaIndice != reg.ImprontaIndice || di.HashGrammatica != reg.HashGrammatica || di.Contesto.Impronta != c.Impronta ||
		!reflect.DeepEqual(di.Contesto.CopiaCanonica(), copia) || !reflect.DeepEqual(di.Diagnostiche, reg.Diagnostiche) {
		t.Error("due chiamate danno regole diverse")
	}
}

// TestRegoleDiLetturaSenzaContestoApplicato: senza contesto, con un contesto proposto, con un contesto che eredita da un
// padre non fornito (nessuna eredità implicita: non si risolve), con un contesto di un altro thread o con due contesti
// per lo stesso thread non c'è un contesto attivo, e nessun file è nel perimetro né letto con il contesto; quando non si
// applica per un errore, la diagnostica è anche nell'esito. Senza insieme di regole le regole sono vuote; un thread che
// non c'è dà falso.
func TestRegoleDiLetturaSenzaContestoApplicato(t *testing.T) {
	th := scenaCollegamento(t)
	_, hash := insiemeConPartner(t)
	occ := motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}
	proposto := contestoACME(th.ID, hash, occ)
	proposto.Stato = motorea.StatoContestoProposto
	figlio := contestoACME(th.ID, hash, occ)
	figlio.EreditaDa = "ctx-acme-progetto"
	altro := contestoACME(uid(0x7f1), hash, occ)
	doppio := contestoACME(th.ID, hash, occ)
	doppio.ID = "ctx-acme-v1-bis"
	f := fotografia(th)
	for _, c := range []struct {
		nome        string
		contesti    []motorea.ContestoLavorazione
		presente    bool   // c'è un contesto risolto, anche non attivo
		diagnostica string // un codice che le regole e l'esito devono avere
	}{
		{"senza contesto", nil, false, ""},
		{"contesto proposto", []motorea.ContestoLavorazione{proposto}, true, motorea.CodiceContestoNonAttivo},
		{"eredita_da senza il padre", []motorea.ContestoLavorazione{figlio}, false, motorea.CodiceContestoNonValido},
		{"contesto di un altro thread", []motorea.ContestoLavorazione{altro}, false, ""},
		{"due contesti per lo stesso thread", []motorea.ContestoLavorazione{contestoACME(th.ID, hash, occ), doppio}, false, motorea.CodiceContestoNonValido},
	} {
		t.Run(c.nome, func(t *testing.T) {
			r, _ := insiemeConPartner(t, c.contesti...)
			et := calcola(t, f, r, valutazione.Ingressi{}).Thread[0]
			reg, ok := valutazione.RegoleDiLettura(f, r, th.ID)
			if !ok || (reg.Contesto != nil) != c.presente || reg.Contesto.Attivo() || len(reg.Contesto.PartnerAttivi()) != 0 {
				t.Fatalf("contesto %+v (presente %v)", reg.Contesto, c.presente)
			}
			if dentro := controllaPerimetro(t, c.nome, reg, et); len(dentro) != 0 {
				t.Errorf("file nel perimetro senza un contesto attivo: %v", dentro)
			}
			if c.diagnostica != "" {
				if len(conCodiceDiag(reg.Diagnostiche, c.diagnostica)) == 0 || len(conCodiceDiag(et.Diagnostiche, c.diagnostica)) == 0 {
					t.Errorf("la diagnostica %s: regole %+v, esito %d diagnostiche", c.diagnostica, reg.Diagnostiche, len(et.Diagnostiche))
				}
			}
			if !sottoinsieme(reg.Diagnostiche, et.Diagnostiche) {
				t.Errorf("diagnostiche esposte fuori dall'esito: %+v", reg.Diagnostiche)
			}
		})
	}

	nessuna, ok := valutazione.RegoleDiLettura(f, nil, th.ID)
	if !ok || nessuna.ImprontaIndice != "" || nessuna.VersioneLimiti != "" || nessuna.HashGrammatica != "" || nessuna.Contesto != nil ||
		len(conCodiceDiag(nessuna.Diagnostiche, motorea.CodiceRegoleAssenti)) != 1 {
		t.Errorf("senza insieme di regole: %+v", nessuna)
	}
	r, _ := insiemeConPartner(t)
	if _, ok := valutazione.RegoleDiLettura(f, r, uid(0x7f2)); ok {
		t.Error("un thread che non c'è nella fotografia dà le regole")
	}
	var vuota fotorfq.Fotografia
	if _, ok := valutazione.RegoleDiLettura(vuota, r, th.ID); ok {
		t.Error("una fotografia vuota dà le regole")
	}
}

// conCodiceDiag: le diagnostiche con quel codice.
func conCodiceDiag(d []evidenze.Diagnostica, codice string) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, x := range d {
		if x.Codice == codice {
			out = append(out, x)
		}
	}
	return out
}
