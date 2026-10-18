package confermabom

// L1 — lo stato della RFQ in memoria (giro 5, A1d, P4, tappa A1, passo B2.3; recuperato in V1, ondata B): versione
// corrente, validita' dell'ultima conferma e controllo della base passano dalle regole pure di firma.go e validita.go,
// senza una seconda definizione. Nessun database.

import (
	"testing"

	"github.com/google/uuid"
)

func TestLoStatoDellaRFQUsaLaFirmaELaValiditaDelPackage(t *testing.T) {
	p, s, x, f2 := idB("p"), idB("s"), idB("x"), idB("f2")
	comp := []Componente{
		{ID: p, CodiceLetterale: "ACM-1000", Tipo: "finito", StepStrutturale: uuid.NullUUID{UUID: idB("doc"), Valid: true}},
		{ID: s, CodiceLetterale: "ACM-1100", Tipo: "sottoassieme", Rev: "A"},
		{ID: x, CodiceLetterale: "ACM-1101", Tipo: "sciolto"},
		{ID: f2, CodiceLetterale: "ACM-6000", Tipo: "finito"},
	}
	archi := []Arco{{Padre: p, Figlio: s, Qta: 1}, {Padre: s, Figlio: x, Qta: 2, Posizione: "10"}}
	firma, err := FirmaPerimetro(p, comp, archi)
	if err != nil {
		t.Fatal(err)
	}
	// senza conferme: numero 0, la firma del package, la fonte autorizzata
	all := idB("allegato")
	st := NuovoStatoRFQ(idB("rfq"), 12, comp, archi, nil, map[uuid.UUID]FonteAutorizzata{p: {DocumentoID: idB("doc"), AllegatoID: &all, Sha256: "ab"}})
	vc, err := st.VersioneCorrente(p)
	if err != nil {
		t.Fatal(err)
	}
	if vc.ConfermaID != nil || vc.Numero != 0 || vc.Contatore != 12 || vc.FirmaStruttura != firma || vc.FonteAutorizzata == nil || *vc.FonteAutorizzata.AllegatoID != all {
		t.Errorf("versione corrente senza conferme: %+v", vc)
	}
	if st.FonteAutorizzata(f2) != nil {
		t.Error("F2 non ha una fonte autorizzata")
	}
	if _, _, ok, err := st.ValiditaUltima(p); ok || err != nil {
		t.Errorf("validita' senza conferme: %v %v", ok, err)
	}
	// due conferme date in disordine: l'ultima e' la n. 2; valida per contatore e per firma
	c1 := TestataConferma{Conferma: Conferma{ID: idB("c1"), Numero: 1, Contatore: 5, FirmaStruttura: firma}, Prodotto: p, Formato: 1, Criterio: "nessuno"}
	c2 := TestataConferma{Conferma: Conferma{ID: idB("c2"), Numero: 2, Contatore: 12, FirmaStruttura: firma}, Prodotto: p, Formato: 2, Criterio: "step"}
	st = NuovoStatoRFQ(idB("rfq"), 12, comp, archi, []TestataConferma{c2, c1}, nil)
	u, v, ok, err := st.ValiditaUltima(p)
	if err != nil || !ok || u.ID != c2.ID || !v.Valida || !v.PerContatore || !v.PerFirma {
		t.Fatalf("validita' dell'ultima: %+v %+v %v %v", u, v, ok, err)
	}
	if vc, _ := st.VersioneCorrente(p); vc.ConfermaID == nil || *vc.ConfermaID != c2.ID || vc.Numero != 2 {
		t.Errorf("versione corrente: %+v", vc)
	}
	// il contatore si muove per un altro prodotto: decide la firma
	st = NuovoStatoRFQ(idB("rfq"), 13, comp, archi, []TestataConferma{c1, c2}, nil)
	if _, v, _, _ := st.ValiditaUltima(p); !v.Valida || v.PerContatore || !v.PerFirma {
		t.Errorf("contatore mosso altrove: %+v", v)
	}
	// la base di una bozza: c2 col contatore vecchio e la firma giusta vale; la n. 1 no
	if m, err := st.ControllaBase(p, Base{ConfermaID: &c2.ID, Numero: 2, Contatore: 12, Firma: firma}); err != nil || len(m) != 0 {
		t.Errorf("base corrente per firma: %v %v", m, err)
	}
	if m, _ := st.ControllaBase(p, Base{ConfermaID: &c1.ID, Numero: 1, Contatore: 13, Firma: firma}); len(m) != 1 || m[0] != MotivoConfermaCambiata {
		t.Errorf("base sulla conferma vecchia: %v", m)
	}
	// un arco del perimetro cambia: ne' contatore ne' firma
	archi2 := []Arco{{Padre: p, Figlio: s, Qta: 1}, {Padre: s, Figlio: x, Qta: 3}}
	st = NuovoStatoRFQ(idB("rfq"), 14, comp, archi2, []TestataConferma{c1, c2}, nil)
	if _, v, _, _ := st.ValiditaUltima(p); v.Valida {
		t.Errorf("perimetro cambiato: %+v", v)
	}
	if m, _ := st.ControllaBase(p, Base{ConfermaID: &c2.ID, Numero: 2, Contatore: 12, Firma: firma}); len(m) != 1 || m[0] != MotivoStrutturaCambiata {
		t.Errorf("base con la struttura cambiata: %v", m)
	}
	if c, ok := st.Componente(s); !ok || c.Rev != "A" {
		t.Errorf("Componente: %+v %v", c, ok)
	}
}

func idB(nome string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("confermabom-base:"+nome))
}
