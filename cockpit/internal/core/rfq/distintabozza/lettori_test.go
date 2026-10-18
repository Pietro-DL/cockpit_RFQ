package distintabozza

// L1 — i lettori dei formati salvati (V1, passo 2; EM §2.5, §9.1, §9.7): le versioni nei formati 2 e 3 con l'impronta
// ricontrollata, gli snapshot delle conferme nei formati 1, 2 e 3 senza nascondere un contenuto illeggibile, il
// passaggio fra il registro e le righe di distinta_importazione.

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// comeJsonb: il testo che restituirebbe PostgreSQL per un jsonb (spazi dopo «:» e «,»), per le riletture.
func comeJsonb(t *testing.T, can []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(can, &v); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestV1ILettoriDeiFormati(t *testing.T) {
	id := idSintetici3()
	s3 := leggiProva3(t, snapshotProva3(id, componiProva3(t, id).Impronta))
	can3, _ := s3.Canonico()
	imp3 := jsoncanonico.Impronta(can3)
	l, err := RileggiVersione(FormatoSnapshot3, comeJsonb(t, can3), imp3)
	if err != nil {
		t.Fatalf("formato 3: %v", err)
	}
	if l.Formato3 == nil || l.Formato2 != nil || !bytes.Equal(l.Canonico, can3) || l.Impronta != imp3 {
		t.Fatalf("formato 3 riletto: %+v", l)
	}
	s2 := Snapshot{Formato: 2, ProdottoID: id.prodotto, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Nodi: []Nodo{{ChiaveBozza: "r", ComponenteID: pu(id.prodotto), Op: OpRiusa, Codice: "ACM-1000", Tipo: "finito", Origine: "registrato"}}}
	can2, _ := s2.Canonico()
	imp2 := jsoncanonico.Impronta(can2)
	l2, err := RileggiVersione(FormatoSnapshot, comeJsonb(t, can2), imp2)
	if err != nil {
		t.Fatalf("formato 2: %v", err)
	}
	if l2.Formato2 == nil || l2.Formato3 != nil || !bytes.Equal(l2.Canonico, can2) {
		t.Fatalf("formato 2 riletto: %+v", l2)
	}
	for nome, c := range map[string]struct {
		formato  int
		raw      []byte
		impronta string
	}{
		"impronta diversa":                 {FormatoSnapshot3, can3, strings.Repeat("0", 64)},
		"formato 3 sotto una testata 2":    {FormatoSnapshot, can3, imp3},
		"formato 2 sotto una testata 3":    {FormatoSnapshot3, can2, imp2},
		"testata formato 1":                {FormatoSnapshot1, []byte(`{"formato":1,"perimetro":{}}`), strings.Repeat("0", 64)},
		"snapshot formato 3 non leggibile": {FormatoSnapshot3, []byte(`{"formato":3,"prodotto_id":"` + id.prodotto.String() + `","nodi":[]}`), imp3},
	} {
		if _, err := RileggiVersione(c.formato, c.raw, c.impronta); !errors.Is(err, ErrSnapshotIncoerente) {
			t.Errorf("%s: atteso ErrSnapshotIncoerente, %v", nome, err)
		}
	}
}

func TestV1LoStoricoNonNascondeNiente(t *testing.T) {
	id := idSintetici3()
	f1 := []byte(`{"perimetro": {"componenti": [], "archi": []}, "formato": 1}`)
	canF1, _ := jsoncanonico.Codifica(json.RawMessage(f1))
	l := LeggiSnapshotConferma(FormatoSnapshot1, f1, jsoncanonico.Impronta(canF1))
	if l.NonLeggibile || !bytes.Equal(l.Formato1, canF1) || !bytes.Equal(l.Canonico, canF1) {
		t.Fatalf("formato 1: %+v", l)
	}
	s3 := leggiProva3(t, snapshotProva3(id, componiProva3(t, id).Impronta))
	can3, _ := s3.Canonico()
	if l := LeggiSnapshotConferma(FormatoSnapshot3, comeJsonb(t, can3), jsoncanonico.Impronta(can3)); l.NonLeggibile || l.Formato3 == nil {
		t.Fatalf("formato 3: %+v", l)
	}
	for nome, c := range map[string]struct {
		formato  int
		raw      []byte
		impronta string
	}{
		"formato 1 con un'altra impronta":   {FormatoSnapshot1, f1, strings.Repeat("0", 64)},
		"formato 1 con un campo in piu'":    {FormatoSnapshot1, []byte(`{"formato":1,"perimetro":{},"mappa":{}}`), strings.Repeat("0", 64)},
		"formato 1 senza perimetro":         {FormatoSnapshot1, []byte(`{"formato":1}`), strings.Repeat("0", 64)},
		"formato 3 con un'altra impronta":   {FormatoSnapshot3, can3, strings.Repeat("0", 64)},
		"formato 3 sotto una conferma 2":    {FormatoSnapshot, can3, jsoncanonico.Impronta(can3)},
		"formato sconosciuto":               {4, []byte(`{"formato":4}`), strings.Repeat("0", 64)},
		"un testo che non e' JSON":          {FormatoSnapshot3, []byte(`{"formato":3,`), strings.Repeat("0", 64)},
		"formato 3 con un nodo senza forma": {FormatoSnapshot3, []byte(`{"formato":3,"prodotto_id":"` + id.prodotto.String() + `","nodi":[{"chiave_bozza":"r"}]}`), strings.Repeat("0", 64)},
	} {
		l := LeggiSnapshotConferma(c.formato, c.raw, c.impronta)
		if !l.NonLeggibile || l.Motivo == "" || len(l.Canonico) == 0 || l.Formato1 != nil || l.Formato2 != nil || l.Formato3 != nil {
			t.Errorf("%s: atteso un contenuto non leggibile con i byte grezzi, %+v", nome, l)
		}
	}
}

func TestV1IlRegistroELeRighe(t *testing.T) {
	id := idSintetici3()
	reg := componiProva3(t, id)
	p := ParametriImportazione(reg, id.utente)
	if p.ThreadID != id.thread || p.ProdottoID != id.prodotto || p.Impronta != reg.Impronta || p.Gruppo != gruppoProva3 ||
		!p.FonteAllegatoID.Valid || p.FonteAllegatoID.UUID != id.allegato || p.FonteSha256.String != id.sha ||
		p.FonteRadice.String != id.rif("#1") || len(p.AllegatiCitati) != 2 || p.FormatoRegistro != 1 ||
		p.VersioneImportazione != importazione.VersioneImportazione1 || !bytes.Equal(p.Contenuto, reg.Canonico) || p.CreataDa != id.utente {
		t.Fatalf("parametri: %+v", p)
	}
	riga := db.GetDistintaImportazioneRow{ImportazioneID: uuid.MustParse("00000000-0000-4000-8000-0000002301a1"), ThreadID: p.ThreadID,
		ProdottoID: p.ProdottoID, Impronta: p.Impronta, ImprontaContenuto: p.ImprontaContenuto, ImprontaEvidenze: p.ImprontaEvidenze,
		ImprontaRegole: p.ImprontaRegole, FormatoRegistro: p.FormatoRegistro, VersioneImportazione: p.VersioneImportazione,
		Gruppo: p.Gruppo, FonteAllegatoID: p.FonteAllegatoID, FonteSha256: p.FonteSha256, FonteRadice: p.FonteRadice,
		AllegatiCitati: []uuid.UUID{p.AllegatiCitati[1], p.AllegatiCitati[0]}, Contenuto: comeJsonb(t, p.Contenuto),
		CreataDa: id.utente, CreataIl: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	letto, testata, err := RileggiImportazione(riga)
	if err != nil {
		t.Fatalf("rilettura: %v", err)
	}
	if !bytes.Equal(letto.Canonico, reg.Canonico) || letto.Impronta != reg.Impronta || testata.ImportazioneID != riga.ImportazioneID {
		t.Fatal("la rilettura dalla riga cambia il registro")
	}
	blocco := ImportazioneLettaDa(letto, testata, false)
	if blocco.ImportazioneID != riga.ImportazioneID || blocco.ImprontaContenuto != reg.Contenuto.ImprontaContenuto ||
		!bytes.Contains(blocco.Regole, []byte(`"importazione":"importazione-1"`)) || blocco.Contenuto != nil {
		t.Fatalf("blocco importazione di R1: %+v", blocco)
	}
	if conContenuto := ImportazioneLettaDa(letto, testata, true); !bytes.Equal(conContenuto.Contenuto, reg.Canonico) {
		t.Fatal("con ?importazione=1 il contenuto manca o cambia")
	}
	guasta := riga
	guasta.FonteRadice = pgtype.Text{String: id.rif("#2"), Valid: true}
	if _, _, err := RileggiImportazione(guasta); !errors.Is(err, importazione.ErrRegistroIncoerente) {
		t.Fatalf("colonna diversa dal contenuto: %v", err)
	}
	guasta = riga
	guasta.Contenuto = bytes.Replace(riga.Contenuto, []byte(`"ACM-1100"`), []byte(`"ACM-1101"`), 1)
	if _, _, err := RileggiImportazione(guasta); !errors.Is(err, importazione.ErrRegistroIncoerente) {
		t.Fatalf("contenuto ritoccato: %v", err)
	}
}
