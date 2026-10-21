package distintabozza

// L1 — i ritocchi del DTO dell'ondata B (decisioni [T] dell'integratore sulle proposte dell'ondata A, 18/10):
//   - PE-27: da_decisione in scollega_documento, un'esclusione dello stesso snapshot e dello stesso componente;
//   - PE-11: una rev vuota vale assente e non si scrive (nodo e decisione identita);
//   - PE-17: segnale_visto.versione_servizio = regole.versioni.ancoraggio del registro (V-5);
//   - PE-22: nodo_escluso nell'elenco chiuso dei motivi di file_svincolato;
//   - PE-05: il prima di annulla e rifai, null oppure {azione_id}.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

func TestV1PE27LoSvincoloCitaLEsclusione(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	escludi := map[string]any{"id": "ex-0001", "tipo": "escludi", "componente_id": "00000000-0000-4000-8000-000000002200", "motivo": "non serve piu'"}
	con := func(t *testing.T, mod func(sv map[string]any)) map[string]any {
		s := copiaProfonda(t, base)
		s["decisioni"] = append(s["decisioni"].([]any), escludi)
		mod(decisioneIn3(s, "sv-0001"))
		return s
	}
	t.Run("valida", func(t *testing.T) {
		s := con(t, func(sv map[string]any) { sv["da_decisione"] = "ex-0001" })
		got, err := LeggiSnapshot3(snapshotJSON(t, s))
		if err != nil {
			t.Fatalf("lo svincolo che cita l'esclusione dello stesso componente e' rifiutato: %v", err)
		}
		if d := decisioneDi3(got, "sv-0001"); d.DaDecisione != "ex-0001" {
			t.Fatalf("da_decisione perso: %+v", d)
		}
		can, err := got.Canonico()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(can, []byte(`"da_decisione":"ex-0001"`)) {
			t.Fatalf("da_decisione non e' nei byte canonici")
		}
	})
	for nome, c := range map[string]struct {
		mod func(s map[string]any)
	}{
		"l'id non c'e'":        {func(s map[string]any) { decisioneIn3(s, "sv-0001")["da_decisione"] = "ex-9999" }},
		"non e' un'esclusione": {func(s map[string]any) { decisioneIn3(s, "sv-0001")["da_decisione"] = "dv-0001" }},
		"esclusione di un altro componente": {func(s map[string]any) {
			decisioneIn3(s, "sv-0001")["da_decisione"] = "ex-0001"
			decisioneIn3(s, "ex-0001")["componente_id"] = "00000000-0000-4000-8000-000000002300"
		}},
		"forma dell'id": {func(s map[string]any) { decisioneIn3(s, "sv-0001")["da_decisione"] = "ex 1" }},
	} {
		t.Run(nome, func(t *testing.T) {
			s := con(t, func(map[string]any) {})
			c.mod(s)
			_, err := LeggiSnapshot3(snapshotJSON(t, s))
			var e *Errore
			if err == nil || !errorsAs(err, &e) || e.Codice != CodiceCorpoNonValido || !strings.HasSuffix(e.Campo, "da_decisione") {
				t.Fatalf("atteso 422 CORPO_NON_VALIDO su da_decisione, avuto %v", err)
			}
		})
	}
	t.Run("lo svincolo senza da_decisione resta valido", func(t *testing.T) {
		if _, err := LeggiSnapshot3(snapshotJSON(t, base)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("da_decisione solo in scollega_documento", func(t *testing.T) {
		s := copiaProfonda(t, base)
		decisioneIn3(s, "dv-0001")["da_decisione"] = "ex-0001"
		if _, err := LeggiSnapshot3(snapshotJSON(t, s)); err == nil {
			t.Fatal("da_decisione accettato in una decisione da_valutare")
		}
	})
}

func errorsAs(err error, e **Errore) bool {
	x, ok := err.(*Errore)
	if ok {
		*e = x
	}
	return ok
}

func TestV1PE11LaRevVuotaValeAssente(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	senza := copiaProfonda(t, snapshotProva3(id, imp))
	conVuota := copiaProfonda(t, senza)
	nodoIn3(conVuota, "k-man")["identita"].(map[string]any)["rev"] = ""
	nulla := copiaProfonda(t, senza)
	nodoIn3(nulla, "k-man")["identita"].(map[string]any)["rev"] = nil
	attesi, err := LeggiSnapshot3(snapshotJSON(t, senza))
	if err != nil {
		t.Fatal(err)
	}
	canAtteso, err := attesi.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	for nome, s := range map[string]map[string]any{"vuota": conVuota, "null": nulla} {
		got, err := LeggiSnapshot3(snapshotJSON(t, s))
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		can, err := got.Canonico()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(can, canAtteso) {
			t.Fatalf("rev %s: i byte canonici non sono quelli del nodo senza rev", nome)
		}
		if n := nodoDi3(got, "k-man"); n.Identita.Rev != "" {
			t.Fatalf("rev %s letta come %q", nome, n.Identita.Rev)
		}
	}
	// la decisione identita: una rev vuota della sua identita' non si scrive
	d := decisioniValide()[DecisioneIdentita]
	d["identita"].(map[string]any)["rev"] = ""
	got, e := leggiDecisioneProva(t, d)
	if e != nil {
		t.Fatal(e)
	}
	can, err := got.Canonica()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(can, []byte(`"rev"`)) {
		t.Fatalf("la rev vuota della decisione identita e' scritta: %s", can)
	}
	// e il nodo in modo proposta la confronta come assente con la rev_proposta vuota del registro
	v := importazione.Voce{Voce: "n:x", CodiceProposto: "ACM-1", IdentitaProposta: json.RawMessage(`{"emittente":"` + id.clienteE() + `","namespace":"acme"}`)}
	idp, ok, err := IdentitaDellaProposta(v)
	if err != nil || !ok || idp.Rev != "" {
		t.Fatalf("identita' della proposta con rev_proposta vuota: %+v %v %v", idp, ok, err)
	}
}

func TestV1PE17LaVersioneDelServizioVieneDalRegistro(t *testing.T) {
	id := idSintetici3()
	casella := Casella3{AllegatoID: id.allegato, Gruppo: gruppoProva3, Rif: id.rif("#31")}
	decisione := func(versione string) Decisione3 {
		d := Decisione3{ID: "ni-1", Tipo: DecisioneNonIncludere, Casella: &casella, MotivoCodice: "elemento_cad", Motivo: "cordone",
			Importazione: strings.Repeat("6", 64)}
		if versione != "" {
			d.SegnaleVisto = &importazione.SegnaleVisto{Codice: "ancoraggio.possibile_elemento_cad", Regola: "elemento_cad_nome_foglia_v1",
				VersioneServizio: versione}
		}
		return d
	}
	for _, x := range []struct {
		ancoraggio, visto string
		ok                bool
	}{
		{"ancoraggio-3", "ancoraggio-3", true},
		{"ancoraggio-4", "ancoraggio-4", true},
		{"ancoraggio-4", "ancoraggio-3", false}, // la versione del servizio e' quella del registro, non un'altra
		{"ancoraggio-3", "", false},             // il registro segnala il rif: la decisione porta il segnale
	} {
		c := registroProva3(t, id)
		c.Regole.Versioni.Ancoraggio = x.ancoraggio
		r, err := importazione.Componi(c)
		if err != nil {
			t.Fatal(err)
		}
		attesi, err := SegnaliAttesi(&r.Contenuto, casella)
		if err != nil || len(attesi) != 1 || attesi[0].VersioneServizio != x.ancoraggio {
			t.Fatalf("segnali attesi con ancoraggio %s: %+v %v", x.ancoraggio, attesi, err)
		}
		e := VerificaSegnaleVisto("decisioni[0]", &r.Contenuto, decisione(x.visto))
		if (e == nil) != x.ok {
			t.Fatalf("ancoraggio %s, visto %q: esito %v, atteso ok = %v", x.ancoraggio, x.visto, e, x.ok)
		}
		if e != nil && e.Motivo != MotivoSegnaleNonCorrisponde {
			t.Fatalf("motivo %q", e.Motivo)
		}
	}
	// un rif che il registro non segnala: la decisione non porta il segnale
	r := componiProva3(t, id)
	senza := Casella3{AllegatoID: id.allegato, Gruppo: gruppoProva3, Rif: id.rif("#9")}
	d := decisione("ancoraggio-3")
	d.Casella = &senza
	if e := VerificaSegnaleVisto("decisioni[0]", &r.Contenuto, d); e == nil || e.Motivo != MotivoSegnaleNonCorrisponde {
		t.Fatalf("segnale su un rif non segnalato: %v", e)
	}
	d.SegnaleVisto = nil
	if e := VerificaSegnaleVisto("decisioni[0]", &r.Contenuto, d); e != nil {
		t.Fatalf("nessun segnale su un rif non segnalato: %v", e)
	}
}

func TestV1PE22IlMotivoNodoEscluso(t *testing.T) {
	if len(MotiviSvincolo) != 6 || !MotivoSvincoloAmmesso(SvincoloNodoEscluso) || !SvincoloNelVassoio(SvincoloNodoEscluso) {
		t.Fatalf("nodo_escluso non e' nell'elenco chiuso o non va nel vassoio: %v", MotiviSvincolo)
	}
	if SvincoloNelVassoio(SvincoloUtente) || SvincoloNelVassoio("escluso") || MotivoSvincoloAmmesso("escluso") {
		t.Fatal("l'elenco non e' chiuso")
	}
	p := "00000000-0000-4000-8000-0000000d0007"
	for _, m := range MotiviSvincolo {
		d, e := LeggiDopoSvincolo("azioni[0].dopo", json.RawMessage(`{"allegato_id":"`+p+`","motivo":"`+m+`"}`))
		if e != nil || d.Motivo != m {
			t.Fatalf("motivo %s rifiutato: %v", m, e)
		}
	}
	for _, raw := range []string{
		`{"allegato_id":"` + p + `","motivo":"escluso"}`,
		`{"allegato_id":"` + p + `"}`,
		`{"allegato_id":"` + p + `","motivo":"nodo_tolto","chiave_bozza":"k-1"}`,
		`null`,
	} {
		if _, e := LeggiDopoSvincolo("azioni[0].dopo", json.RawMessage(raw)); e == nil {
			t.Fatalf("dopo di file_svincolato accettato: %s", raw)
		}
	}
	d, e := LeggiDopoSvincolo("azioni[0].dopo", json.RawMessage(`{"allegato_id":"`+p+`","motivo":"non_incluso","azione_id":"00000000-0000-4000-8000-0000000e0101"}`))
	if e != nil || d.AzioneID == nil {
		t.Fatalf("l'azione_id del gesto cumulativo nel dopo dello svincolo: %v", e)
	}
}

func TestV1PE05IlPrimaDellAnnullamento(t *testing.T) {
	a := "00000000-0000-4000-8000-0000000e0101"
	if x, e := LeggiPrimaAnnullamento("azioni[0].prima", nil); e != nil || x != nil {
		t.Fatalf("prima assente: %v %v", x, e)
	}
	if x, e := LeggiPrimaAnnullamento("azioni[0].prima", json.RawMessage(`null`)); e != nil || x != nil {
		t.Fatalf("prima null: %v %v", x, e)
	}
	x, e := LeggiPrimaAnnullamento("azioni[0].prima", json.RawMessage(`{"azione_id":"`+a+`"}`))
	if e != nil || x == nil || x.String() != a {
		t.Fatalf("prima {azione_id}: %v %v", x, e)
	}
	for _, raw := range []string{
		`{"azione_id":"` + uuid.NewSHA1(uuid.NameSpaceURL, []byte("v5")).String() + `"}`, // non v4
		`{"azione_id":"` + a + `","n":3}`,
		`{}`,
		`{"n":3}`,
	} {
		if _, e := LeggiPrimaAnnullamento("azioni[0].prima", json.RawMessage(raw)); e == nil {
			t.Fatalf("prima dell'annullamento accettato: %s", raw)
		}
	}
}
