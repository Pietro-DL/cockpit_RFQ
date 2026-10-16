package ancoraggio

import (
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// GP-1 (addendum «contesto partner» §5-§6): i ruoli delle letture nell'ancoraggio. Codici inventati.

func letturaRuolo(id, famiglia, namespace, base, ruolo string) motorea.LetturaCodice {
	return motorea.LetturaCodice{ID: id, UnitaID: "u:" + id, Ruolo: ruolo, Funzione: motorea.FunzIdentitaFile,
		Forma: motorea.LetturaForma{Famiglia: famiglia, Namespace: namespace,
			Base: motorea.BaseLetta{Originale: base, Normalizzata: base, Completa: true,
				Segmenti: []motorea.SegmentoLetto{{Nome: "n", Originale: base, Normalizzato: base}}}}}
}

// Un riferimento sul nodo non è l'identità e non discorda con la primaria; due primarie incompatibili, anche in
// namespace diversi, restano discordanti (T-E1-07).
func TestGP1LetturaDelNodoConRiferimento(t *testing.T) {
	p := letturaRuolo("l1", "acme-punti", "acme-punti", "91.8888.7777", "")
	r := letturaRuolo("l2", "beta-dis", "beta-dis", "654301", grammatica.RuoloLetturaRiferimento)
	if l, motivo := letturaDelNodo([]motorea.LetturaCodice{r, p}); l == nil || l.ID != "l1" || motivo != "" {
		t.Errorf("primaria scelta e riferimento elencato a parte: %+v %q", l, motivo)
	}
	q := letturaRuolo("l3", "beta-dis", "beta-dis", "654301", "")
	if l, motivo := letturaDelNodo([]motorea.LetturaCodice{p, q}); l != nil || motivo != MotivoLettureDiscordanti {
		t.Errorf("due primarie incompatibili restano discordanti (T-E1-07): %+v %q", l, motivo)
	}
	if l, motivo := letturaDelNodo([]motorea.LetturaCodice{r}); l != nil || motivo != MotivoLetturaAssente {
		t.Errorf("un riferimento da solo non diventa mai identità: %+v %q", l, motivo)
	}
}

// Due varianti della stessa tavola non discordano; due primarie con basi diverse sì.
func TestGP1VariantiNonDiscordano(t *testing.T) {
	a := letturaRuolo("v1", "beta-dis", "beta-dis", "765400", grammatica.RuoloLetturaVariante)
	b := letturaRuolo("v2", "beta-dis", "beta-dis", "765401", grammatica.RuoloLetturaVariante)
	if discordantiFra([]motorea.LetturaCodice{a, b}) {
		t.Error("le varianti di un legame non sono discordanti")
	}
	a.Ruolo, b.Ruolo = "", ""
	if !discordantiFra([]motorea.LetturaCodice{a, b}) {
		t.Error("due primarie con basi diverse restano discordanti")
	}
}

// lettureIdentita toglie i riferimenti e tiene le varianti; DividiRiferimenti li separa.
func TestGP1LettureIdentitaSenzaRiferimenti(t *testing.T) {
	r := motorea.Interpretazione{Letture: []motorea.LetturaCodice{
		letturaRuolo("a", "acme-punti", "acme-punti", "91.8888.7777", ""),
		letturaRuolo("b", "beta-dis", "beta-dis", "654301", grammatica.RuoloLetturaRiferimento),
		letturaRuolo("c", "beta-dis", "beta-dis", "765400", grammatica.RuoloLetturaVariante),
	}}
	got := lettureIdentita(r)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Errorf("identità: %+v", got)
	}
	id, rif := DividiRiferimenti(r.Letture)
	if len(id) != 2 || len(rif) != 1 || rif[0].ID != "b" {
		t.Errorf("divisione: %+v / %+v", id, rif)
	}
}
