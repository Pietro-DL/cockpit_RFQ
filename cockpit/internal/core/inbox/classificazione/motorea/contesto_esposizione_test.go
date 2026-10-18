package motorea

import (
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L1 — le esposizioni del contesto risolto per il registro d'importazione (V1: emendamento di V1 §3.8 «regole» e
// «contesto», §3.8.2; piano della 0023 §14 passo 6, RM-13): Attivo, PartnerAttivi, CopiaCanonica, NelPerimetro. Ogni
// esposizione dice quello che Interpreta fa, con lo stesso criterio: il perimetro esposto coincide con l'attivazione
// (ContestoID e ImprontaContesto dell'interpretazione), i partner attivi con l'hash delle loro letture, la copia
// canonica con l'impronta del contesto. Le esposizioni non cambiano la lettura: l'impronta è quella di prima. Tutto è
// inventato, come in partner_test.go: ACME il cliente, BETA il fornitore.

// documentiPerimetro: i documenti delle prove del perimetro, con l'occorrenza che li distingue.
func documentiPerimetro(t *testing.T) map[string]evidenze.DocumentoEvidenze {
	t.Helper()
	altroSha := strings.Repeat("2", 64)
	return map[string]evidenze.DocumentoEvidenze{
		"nel perimetro per sha e nome": docAllegato(t, "654321.pdf", shaP1, nil, uNome(t, "654321.pdf")),
		"stesso sha, un altro nome":    docAllegato(t, "654322.pdf", shaP1, nil, uNome(t, "654322.pdf")),
		"stesso nome, un altro sha":    docAllegato(t, "654321.pdf", altroSha, nil, uNome(t, "654321.pdf")),
	}
}

// TestContestoNelPerimetroComeLAttivazione: per ogni documento, NelPerimetro è vero se e solo se Interpreta lo legge con
// il contesto (ContestoID e ImprontaContesto dell'interpretazione sono quelli del contesto); per allegato, per sha256 e
// nome, mai per sola impronta; con un contesto proposto niente è nel perimetro, e niente si attiva.
func TestContestoNelPerimetroComeLAttivazione(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	contesti := map[string]ContestoLavorazione{
		"per sha e nome": contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "654321.pdf"}}, nil),
		"per allegato":   contesto(hash, []OccorrenzaContesto{{AllegatoID: "00000000-0000-4000-8000-0000000000f1"}}, nil),
	}
	proposto := contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "654321.pdf"}}, nil)
	proposto.Stato = StatoContestoProposto
	contesti["proposto"] = proposto
	visti := map[string]int{}
	for nomeC, c := range contesti {
		r, _, err := RisolviContesto(m, c, nil)
		if err != nil {
			t.Fatalf("%s: %v", nomeC, err)
		}
		if r.Attivo() != (c.Stato == StatoContestoDichiarato) {
			t.Errorf("%s: Attivo %v con lo stato %s", nomeC, r.Attivo(), c.Stato)
		}
		mc := m.ConContesto(r)
		for nomeD, d := range documentiPerimetro(t) {
			i := interpreta(t, mc, d, evidenze.UsoSconosciuto(d.BundleID))
			letto := i.ContestoID != ""
			if r.NelPerimetro(d) != letto {
				t.Errorf("%s / %s: NelPerimetro %v, ma l'interpretazione ha il contesto %q", nomeC, nomeD, r.NelPerimetro(d), i.ContestoID)
			}
			if letto {
				visti[nomeC]++
				if i.ContestoID != r.ID || i.ImprontaContesto != r.Impronta {
					t.Errorf("%s / %s: il contesto dell'interpretazione %q %q, quello risolto %q %q", nomeC, nomeD, i.ContestoID, i.ImprontaContesto, r.ID, r.Impronta)
				}
			}
		}
	}
	// per sha e nome: solo il primo documento; per allegato: tutti i documenti dello stesso allegato (anche con un
	// altro nome o un altro sha: l'allegato è l'occorrenza); proposto: nessuno.
	if want := map[string]int{"per sha e nome": 1, "per allegato": 3}; !reflect.DeepEqual(visti, want) {
		t.Errorf("documenti letti con il contesto %v, attesi %v", visti, want)
	}
	// Un documento che non viene da un allegato (un messaggio) non è mai nel perimetro, come per l'attivazione.
	r, _, err := RisolviContesto(m, contesti["per allegato"], nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := docAllegato(t, "654321.pdf", shaP1, nil, uNome(t, "654321.pdf"))
	msg.Fonte.Tipo = "messaggio"
	if r.NelPerimetro(msg) || r.attivazionePer(msg) != nil {
		t.Error("un documento che non viene da un allegato è nel perimetro")
	}
}

// TestContestoPartnerAttivi: i partner attivi sono quelli che il contesto attiva e il motore ha caricato, con l'hash
// della grammatica caricata, lo stesso delle loro letture; un partner con un hash che il motore non ha non c'è (il
// contesto resta attivo e parziale); un contesto proposto non ne attiva nessuno; nil non ne ha.
func TestContestoPartnerAttivi(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	r, _, err := RisolviContesto(m, contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "654321.pdf"}}, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.PartnerAttivi(), []PartnerAttivo{{Emittente: "beta", Hash: hash}}; !reflect.DeepEqual(got, want) {
		t.Errorf("partner attivi %+v, attesi %+v", got, want)
	}
	i := interpreta(t, m.ConContesto(r), docAllegato(t, "654321.pdf", shaP1, nil, uNome(t, "654321.pdf")), evidenze.UsoSconosciuto("bundle-gp1-654321.pdf"))
	if b := lettureDi(i, "beta-dis"); len(b) != 1 || b[0].Forma.Emittente != "beta" || b[0].Forma.HashEmittente != r.PartnerAttivi()[0].Hash {
		t.Errorf("l'hash del partner attivo è quello delle sue letture: %+v", b)
	}

	nc, diag, err := RisolviContesto(m, contesto(strings.Repeat("9", 64), []OccorrenzaContesto{{Sha256: shaP1, Nome: "654321.pdf"}}, nil), nil)
	if err != nil || len(conCodice(diag, CodiceContestoPartnerNonCaricabile)) != 1 {
		t.Fatalf("partner non caricabile: %v\n%s", err, elenco(diag))
	}
	if !nc.Attivo() || len(nc.PartnerAttivi()) != 0 {
		t.Errorf("un partner non caricabile non è attivo: attivo %v, partner %+v", nc.Attivo(), nc.PartnerAttivi())
	}

	p := contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "654321.pdf"}}, nil)
	p.Stato = StatoContestoRevocato
	pr, _, err := RisolviContesto(m, p, nil)
	if err != nil || pr.Attivo() || pr.PartnerAttivi() != nil {
		t.Errorf("un contesto revocato non attiva partner: %v %v %+v", err, pr.Attivo(), pr.PartnerAttivi())
	}

	var vuoto *ContestoRisolto
	if vuoto.Attivo() || vuoto.PartnerAttivi() != nil || vuoto.CopiaCanonica() != nil ||
		vuoto.NelPerimetro(docAllegato(t, "654321.pdf", shaP1, nil, uNome(t, "654321.pdf"))) {
		t.Error("un contesto nil non espone niente")
	}
}

// TestContestoCopiaCanonica: la copia canonica è il contesto normalizzato, con il padre di eredita_da quando c'è; la sua
// impronta è quella del contesto risolto, e anche quella che si calcolava prima di V1 con il tipo senza nome (le
// esposizioni non cambiano la lettura); una copia cambiata non tocca la copia successiva.
func TestContestoCopiaCanonica(t *testing.T) {
	m, hash, _ := motorePartner(t, "")
	padre := contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "b.pdf"}, {Sha256: shaP1, Nome: "a.pdf"}}, nil)
	padre.ID, padre.RFQ, padre.Workspace = "ctx-acme-progetto", "progetto-acme", "progetto-acme"
	figlio := contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "654321.pdf"}}, nil)
	figlio.Partner, figlio.EreditaDa = nil, padre.ID
	for nome, caso := range map[string]struct {
		c     ContestoLavorazione
		padre *ContestoLavorazione
	}{"senza eredità": {contesto(hash, []OccorrenzaContesto{{Sha256: shaP1, Nome: "b.pdf"}, {Sha256: shaP1, Nome: "a.pdf"}}, nil), nil},
		"con eredita_da": {figlio, &padre}} {
		r, _, err := RisolviContesto(m, caso.c, caso.padre)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		copia := r.CopiaCanonica()
		if imp, err := jsoncanonico.ImprontaDi(copia); err != nil || imp != r.Impronta {
			t.Errorf("%s: l'impronta della copia %s, quella del contesto %s (%v)", nome, imp, r.Impronta, err)
		}
		var np *ContestoLavorazione
		if caso.padre != nil {
			x := normalizzaContesto(*caso.padre)
			np = &x
		}
		prima, err := jsoncanonico.ImprontaDi(struct {
			Contesto ContestoLavorazione  `json:"contesto"`
			Padre    *ContestoLavorazione `json:"padre,omitempty"`
		}{normalizzaContesto(caso.c), np})
		if err != nil || prima != r.Impronta {
			t.Errorf("%s: l'impronta di prima di V1 %s, quella di oggi %s: le esposizioni hanno cambiato la lettura", nome, prima, r.Impronta)
		}
		if (copia.Padre != nil) != (caso.padre != nil) || copia.Contesto.ID != caso.c.ID {
			t.Errorf("%s: copia %+v", nome, copia)
		}
		if caso.padre != nil && (copia.Padre.ID != padre.ID || copia.Contesto.EreditaDa != padre.ID) {
			t.Errorf("%s: il padre nella copia %+v", nome, copia.Padre)
		}
		if p := copia.Contesto.Perimetro; len(p) == 0 || (len(p) == 2 && p[0].Nome != "a.pdf") {
			t.Errorf("%s: il perimetro della copia non è normalizzato: %+v", nome, p)
		}
		copia.Contesto.Perimetro[0].Nome = "cambiato.pdf"
		copia.Contesto.ID = "cambiato"
		if di := r.CopiaCanonica(); di.Contesto.ID != caso.c.ID || di.Contesto.Perimetro[0].Nome == "cambiato.pdf" {
			t.Errorf("%s: la copia cambiata ha toccato il contesto: %+v", nome, di.Contesto)
		}
	}
}
