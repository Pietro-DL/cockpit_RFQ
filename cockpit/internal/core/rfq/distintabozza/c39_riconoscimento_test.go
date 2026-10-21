package distintabozza

// L1 — R39-15 (decisione [T-39] dell'orchestratore): il riconoscimento esplicito di una PB legacy con l'azione R2
// file_riconosciuto. La riconciliazione lo accetta solo sulla PB che lo snapshot ha su quel nodo e non cambia lo snapshot
// strutturale; la provenienza resta legacy e la PB passa a verificata (decisa dal server); il riconoscimento di una PB non
// legacy e la verificata dichiarata senza riconoscimento sono rifiutati; spostare la PB la rifa' a mano (manuale, non piu'
// verificata). In R4 la PB verificata non blocca piu' (Piano) e sta nel gruppo legacy_verificata. Dati sintetici.

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
)

func snapshotConPB(t *testing.T, f *fixR4) Snapshot3 {
	t.Helper()
	return f.snapshot()
}

func TestV1D39RiconoscimentoPBLegacy(t *testing.T) {
	f := nuovaFixR4(t)
	f.nodoMan("k-a", "ACM-1100", "sciolto")
	f.arco("k-root", "k-a", 1)
	a := f.occorrenza("ACM-1100.pdf", "ACM-1100")
	b := f.occorrenza("ACM-1100-bis.pdf", "ACM-1100")
	f.preassocia(a, "k-a", ProvenienzaLegacy, "")
	f.preassocia(b, "k-a", ProvenienzaManuale, "")
	prima := snapshotConPB(t, f)
	riconosci := func(al uuid.UUID, chiave string) Azione {
		return Azione{Azione: AzFileRiconosciuto, ChiaveBozza: chiave, Prima: jsonR3915(map[string]any{"allegato_id": al, "chiave_bozza": chiave}),
			Dopo: jsonR3915(map[string]any{"allegato_id": al, "chiave_bozza": chiave, "motivo": "il disegno e' quello del pezzo"}), Origine: "utente"}
	}
	// la riconciliazione: lo snapshot strutturale non cambia, la PB riconosciuta e' nell'elenco
	ric, err := Riconcilia3(prima, senzaProvenienza(prima), []Azione{riconosci(a, "k-a")}, Contesto3{})
	if err != nil || len(ric.Riconosciuti) != 1 || ric.Riconosciuti[0].AllegatoID != a || ric.Strutturali != 0 || len(ric.Eventi) != 1 ||
		ric.Eventi[0].ChiaveBozza != "k-a" {
		t.Fatalf("riconciliazione: %v %+v", err, ric)
	}
	// sul nodo sbagliato o su un file non preassociato: azioni non riconciliate
	for _, x := range []Azione{riconosci(a, "k-root"), riconosci(idDi("altro-file"), "k-a")} {
		if _, err := Riconcilia3(prima, senzaProvenienza(prima), []Azione{x}, Contesto3{}); err == nil {
			t.Fatalf("riconoscimento non riconciliato accettato: %+v", x)
		}
	}
	// annulla di un riconoscimento: non ha un inverso
	if _, _, _, ok := inversaV2(AzFileRiconosciuto, nil, nil); ok {
		t.Fatal("file_riconosciuto si annulla")
	}
	// la provenienza decisa: legacy verificata; la manuale non si riconosce; la verificata senza riconoscimento no
	vf := &verifica3{prima: prima, nuovo: prima}
	finali, err := vf.provenienzeDecise(ric.Riconosciuti)
	if err != nil || len(finali) != 2 || finali[0].Provenienza != ProvenienzaLegacy || !finali[0].Verificata || finali[1].Verificata {
		t.Fatalf("provenienza decisa: %v %+v", err, finali)
	}
	if _, err := vf.provenienzeDecise([]Preassociazione3{{AllegatoID: b, ChiaveBozza: "k-a"}}); err == nil || err.(*Errore).Motivo != MotivoRiconoscimentoNonApplicabile {
		t.Fatalf("riconoscimento di una PB manuale: %v", err)
	}
	dichiarata := prima.copia()
	dichiarata.DocumentiPreassociati[0].Verificata = true
	vf2 := &verifica3{prima: prima, nuovo: dichiarata}
	if _, err := vf2.provenienzeDecise(nil); err == nil || err.(*Errore).Motivo != MotivoProvenienzaDelServer {
		t.Fatalf("verificata dichiarata dalla pagina: %v", err)
	}
	// il salvataggio dopo: la verificata resta finche' la PB resta sul suo nodo
	dopo := prima.copia()
	dopo.DocumentiPreassociati = finali
	vf3 := &verifica3{prima: dopo, nuovo: dopo}
	if ff, err := vf3.provenienzeDecise(nil); err != nil || !ff[0].Verificata {
		t.Fatalf("la verificata non resta: %v %+v", err, ff)
	}
	spostata := dopo.copia()
	spostata.DocumentiPreassociati[0].ChiaveBozza = "k-root"
	spostata.DocumentiPreassociati[0].Verificata = false
	vf4 := &verifica3{prima: dopo, nuovo: spostata}
	if ff, err := vf4.provenienzeDecise(nil); err != nil || ff[0].Provenienza != ProvenienzaManuale || ff[0].Verificata {
		t.Fatalf("spostata: %v %+v", err, ff)
	}
	// la forma: verificata solo con legacy
	if e := validaProvenienza("pb", Preassociazione3{AllegatoID: a, ChiaveBozza: "k-a", Provenienza: ProvenienzaManuale, Verificata: true}); e == nil {
		t.Fatal("verificata su una PB manuale accettata")
	}
	// R4: la legacy non verificata blocca (legacy_da_verificare), la verificata no ed e' nel suo gruppo
	r := f.calcola()
	if bb := trova(r.Blocchi, BloccoPreassociazioneNonApplicab); len(bb) != 1 || bb[0].Motivo != associazioni.MotivoLegacyDaVerificare ||
		len(r.EffettiDocumentali.LegacyDaVerificare) != 1 || len(bb[0].Rimedi) != 3 {
		t.Fatalf("legacy non verificata: %+v", bb)
	}
	f.pb[0].(map[string]any)["verificata"] = true
	r = f.calcola()
	if len(trova(r.Blocchi, BloccoPreassociazioneNonApplicab)) != 0 || len(r.EffettiDocumentali.LegacyVerificate) != 1 ||
		len(r.EffettiDocumentali.LegacyDaVerificare) != 0 || r.DocumentiPreassociati[0].Gruppo != GruppoLegacyVerificata ||
		!r.DocumentiPreassociati[0].Verificata || r.DocumentiPreassociati[0].Esito != associazioni.EsitoApplicabile {
		t.Fatalf("legacy verificata: %v %+v %+v", codici(r.Blocchi), r.EffettiDocumentali, r.DocumentiPreassociati)
	}
}

func jsonR3915(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
