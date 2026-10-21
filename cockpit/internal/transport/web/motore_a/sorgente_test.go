package motore_a

// L1 — V1, ondata C: S9, lo smistamento della Sorgente per tutti i prodotti (emendamento U2 §2.3; prove R-36…R-40, la
// parte L1), sulla scena ACME inventata (indice_test.go):
//   - una sezione per prodotto valutato, nell'ordine; gli STEP dei gruppi con le candidate di ogni prodotto e i nodi
//     segnalati (LS-02 arriva fino a S9); da_associare dai file da smistare del motore, con il loro motivo;
//   - la firma: 64 esadecimali, la stessa a ogni composizione, diversa quando cambia un campo mostrato (un file in Altri
//     file), mai l'ora del calcolo;
//   - un file senza i fatti: «non ancora pronto» (lo STEP in_analisi, il prodotto in_attesa_analisi), mai da associare;
//   - le azioni per stato, sempre {disponibile, motivo}: senza bozza nessuna azione sulla bozza, con il motivo;
//   - i nomi JSON della forma di U2 §2.3.

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestS9LaSorgenteDellaScena(t *testing.T) {
	r := maRisultatoACME()
	s := ComponiSorgente(r, maUUID(1))
	if len(s.Prodotti) != len(r.EsitoThread.ProdottiValutati) {
		t.Fatalf("le sezioni dei prodotti: %d, attese %d", len(s.Prodotti), len(r.EsitoThread.ProdottiValutati))
	}
	for i, p := range s.Prodotti {
		if p.Prodotto != r.EsitoThread.ProdottiValutati[i].Rif || p.Righe == nil || p.InAttesaSorgente == nil || p.Calcolo.FileInAnalisi == nil {
			t.Errorf("la sezione %d: %+v", i, p)
		}
		if p.Calcolo.Stato != CalcoloPronto && p.Calcolo.Stato != CalcoloInAttesaAnalisi && p.Calcolo.Stato != CalcoloErrore {
			t.Errorf("lo stato del calcolo %q", p.Calcolo.Stato)
		}
		if p.Calcolo.Stato == CalcoloInAttesaAnalisi && len(p.Calcolo.FileInAnalisi) == 0 {
			t.Errorf("in attesa dell'analisi senza file: %+v", p.Calcolo)
		}
		t.Logf("%s: conti %+v, righe %d, in attesa della sorgente %d, calcolo %s", p.Prodotto, p.Conti, len(p.Righe), len(p.InAttesaSorgente), p.Calcolo.Stato)
		for _, riga := range p.Righe {
			if (riga.Destinazione == (DestinazioneRiga{}) && (riga.Stato != StatoRigaDaRicollocare || riga.Uscita == nil)) || riga.CondivisoCon == nil ||
				riga.Condivisione == nil {
				t.Errorf("una riga senza riferimento stabile: %+v", riga)
			}
		}
	}
	// gli STEP: ognuno con le candidate dei gruppi dei prodotti che lo hanno, i nodi segnalati come in /dati
	gruppi := 0
	for i := range r.EsitoThread.ProdottiValutati {
		p := ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Prodotto: r.EsitoThread.ProdottiValutati[i].Rif})
		for _, g := range p.Gruppi {
			if g.AllegatoID == nil || g.Tipo == propostabom.GruppoSenzaSTEP {
				continue
			}
			gruppi++
			trovata := false
			for _, st := range s.Step {
				for _, c := range st.Candidate {
					if st.AllegatoID == *g.AllegatoID && c.Prodotto == p.Prodotto && c.Gruppo == g.Chiave {
						trovata = c.Segnalati == g.Segnalati && c.ImprontaContenuto == g.ImprontaContenuto && c.Compatibilita == g.Compatibilita
					}
				}
			}
			if !trovata {
				t.Errorf("il gruppo %s del prodotto %s non è fra le candidate di S9 (o con campi diversi)", g.Chiave, p.Prodotto)
			}
		}
	}
	n := 0
	for _, st := range s.Step {
		n += len(st.Candidate)
	}
	if gruppi == 0 || n != gruppi {
		t.Errorf("le candidate di S9 %d, i gruppi con uno STEP %d", n, gruppi)
	}
	// da associare: i file da smistare del motore con il loro motivo
	if len(s.DaAssociare) != len(r.EsitoThread.DaSmistare) {
		t.Errorf("da associare %d, da smistare %d", len(s.DaAssociare), len(r.EsitoThread.DaSmistare))
	}
	for i, d := range s.DaAssociare {
		if d.AllegatoID != r.EsitoThread.DaSmistare[i].AllegatoID || d.Motivo != string(r.EsitoThread.DaSmistare[i].Motivo) || d.Motivo == "" {
			t.Errorf("da associare %+v", d)
		}
	}

	// la firma
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(s.Firma) {
		t.Fatalf("la firma %q", s.Firma)
	}
	if s2 := ComponiSorgente(maRisultatoACME(), maUUID(1)); s2.Firma != s.Firma {
		t.Errorf("la firma cambia senza cambi: %s / %s", s.Firma, s2.Firma)
	}
	r3 := maRisultatoACME()
	r3.Foto.PresaIl = r3.Foto.PresaIl.Add(3600e9)
	if s3 := ComponiSorgente(r3, maUUID(1)); s3.Firma != s.Firma {
		t.Error("la firma dipende dall'ora del calcolo")
	}
	r4 := maRisultatoACME()
	a := maUUID(98)
	r4.V1.NonAttivi = propostabom.NonAttivi{a: "altri_file"}
	s4 := ComponiSorgente(r4, maUUID(1))
	if s4.Firma == s.Firma || len(s4.AltriFile) != 1 || s4.AltriFile[0].AllegatoID != a {
		t.Errorf("un file in Altri file: firma %s, altri file %+v", s4.Firma, s4.AltriFile)
	}
	// uno STEP senza i fatti (analisi pendente): lo STEP «in_analisi», il prodotto «in_attesa_analisi» con il file, mai
	// fra i file da associare
	r5 := maRisultatoACME()
	if len(s.Step) == 0 {
		t.Fatal("la scena non ha STEP")
	}
	pendente := s.Step[0].AllegatoID
	for i := range r5.EsitoThread.File {
		if r5.EsitoThread.File[i].AllegatoID == pendente {
			r5.EsitoThread.File[i].Disponibilita = ancoraggio.DisponibilitaPendente
		}
	}
	s5 := ComponiSorgente(r5, maUUID(1))
	for _, st := range s5.Step {
		if st.AllegatoID == pendente && st.Analisi != AnalisiInCorso {
			t.Errorf("lo STEP pendente: %+v", st)
		}
	}
	attesa := 0
	for _, p := range s5.Prodotti {
		if p.Calcolo.Stato == CalcoloInAttesaAnalisi && slices.Contains(p.Calcolo.FileInAnalisi, pendente) && p.Calcolo.Motivo == MotivoCalcoloFileInAnalisi {
			attesa++
		}
	}
	if attesa == 0 {
		t.Errorf("nessun prodotto in attesa dell'analisi con lo STEP pendente: %+v", s5.Prodotti)
	}
	for _, d := range s5.DaAssociare {
		if d.AllegatoID == pendente {
			t.Errorf("lo STEP pendente fra i file da associare: %+v", d)
		}
	}
	// un risultato vuoto: gli elenchi ci sono, vuoti
	if v := ComponiSorgente(nil, maUUID(1)); v.Step == nil || v.Prodotti == nil || v.DaAssociare == nil || v.AltriFile == nil {
		t.Errorf("senza risultato: %+v", v)
	}
}

func TestS9LeAzioniPerStato(t *testing.T) {
	ogni := func(a AzioniRiga) []AzioneSorgente {
		return []AzioneSorgente{a.SalvaNellaBozza, a.TogliDallaBozza, a.Sposta, a.ScollegaAllaConferma}
	}
	for _, stato := range []string{StatoRigaProposto, StatoRigaInConflitto, StatoRigaRegistrato, StatoRigaSalvato, StatoRigaDaScollegare,
		StatoRigaDaRicollocare, StatoRigaLegatoComeSorgente, StatoRigaLegameStorico} {
		for _, conBozza := range []bool{false, true} {
			for i, x := range ogni(azioniDellaRiga(stato, conBozza)) {
				if x.Disponibile == (x.Motivo != "") {
					t.Errorf("%s (bozza %v), azione %d: %+v: o disponibile o con il motivo", stato, conBozza, i, x)
				}
			}
		}
	}
	for _, stato := range []string{StatoRigaProposto, StatoRigaInConflitto, StatoRigaRegistrato} {
		for i, x := range ogni(azioniDellaRiga(stato, false)) {
			if x.Disponibile {
				t.Errorf("%s senza bozza, azione %d disponibile", stato, i)
			}
		}
	}
	if a := azioniDellaRiga(StatoRigaProposto, true); !a.SalvaNellaBozza.Disponibile || !a.Sposta.Disponibile || a.ScollegaAllaConferma.Disponibile {
		t.Errorf("proposto con la bozza: %+v", a)
	}
	if a := azioniDellaRiga(StatoRigaRegistrato, true); a.SalvaNellaBozza.Disponibile || !a.ScollegaAllaConferma.Disponibile {
		t.Errorf("registrato con la bozza: %+v", a)
	}
	if a := azioniDellaRiga(StatoRigaInConflitto, true); a.SalvaNellaBozza.Disponibile || !strings.Contains(a.SalvaNellaBozza.Motivo, "conflitto") {
		t.Errorf("in conflitto con la bozza: %+v", a)
	}
}

func TestS9INomiDeiCampi(t *testing.T) {
	nomi := func(v any) []string {
		var out []string
		tipo := reflect.TypeOf(v)
		for i := range tipo.NumField() {
			if n, _, _ := strings.Cut(tipo.Field(i).Tag.Get("json"), ","); n != "-" && n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	for _, c := range []struct {
		v      any
		atteso []string
	}{
		{Sorgente{}, []string{"firma", "step", "prodotti", "da_associare", "altri_file", "calcolato"}},
		{StepDellaSorgente{}, []string{"allegato_id", "nome_file", "analisi", "candidate"}},
		{CandidataDelloStep{}, []string{"prodotto_id", "prodotto", "gruppo", "compatibilita", "radice_letterale", "struttura", "segnalati",
			"impronta_contenuto", "sorgente_di"}},
		{ProdottoSorgente{}, []string{"prodotto_id", "prodotto", "codice", "identificativo_cliente", "fonte", "bozza", "calcolo", "conti", "righe",
			"in_attesa_sorgente", "preassociazioni_proposte"}},
		// la forma di R1 (una sola forma per S9 e R1, direttiva 39 §4 [T-39]): chiave_bozza sempre, importazione = l'impronta
		{distintabozza.PreassociazioneProposta3{}, []string{"allegato_id", "chiave_bozza", "provenienza", "importazione", "motivo"}},
		{FonteSorgente{}, []string{"allegato_id", "confermata", "confermata_il", "confermata_da", "autorizzata", "autorizzata_il", "autorizzata_da",
			"autorizzata_motivo", "motivo", "conferma_ammessa", "gesto", "sorgente_confermata", "sorgente_s8", "legame_storico", "struttura_step",
			"proposte_sorgente"}},
		{BozzaDellaSorgente{}, []string{"bozza_id", "versione", "sorgente"}},
		{CalcoloDelProdotto{}, []string{"stato", "motivo", "file_in_analisi"}},
		{ContiDelProdotto{}, []string{"proposti", "salvati", "registrati", "confermati", "da_scollegare", "da_ricollocare", "in_conflitto",
			"nodi_senza_file", "legati_come_sorgente", "legami_storici"}},
		{RigaSorgente{}, []string{"allegato_id", "proposta_id", "destinazione", "uscita", "codice_mostrato", "rev_mostrata", "identificativo_cliente",
			"stato", "motivo", "origine", "provenienza", "priorita_anteprima", "condiviso_con", "condivisione", "azioni"}},
		{CondivisioneRiga{}, []string{"prodotto_id", "motivo"}},
		{DestinazioneRiga{}, []string{"chiave_bozza", "casella", "voce", "componente_id"}},
		{CasellaRiga{}, []string{"allegato_id", "gruppo", "rif"}},
		{AzioniRiga{}, []string{"salva_nella_bozza", "togli_dalla_bozza", "sposta", "scollega_alla_conferma"}},
		{AzioneSorgente{}, []string{"disponibile", "motivo"}},
		{FileInAttesaSorgente{}, []string{"allegato_id", "voce", "codice_mostrato", "motivo"}},
		{FileDaAssociare{}, []string{"allegato_id", "motivo"}},
		{FileInAltriFile{}, []string{"allegato_id", "nome_sul_nas", "stato_copia"}},
	} {
		if got := nomi(c.v); !slices.Equal(got, c.atteso) {
			t.Errorf("%T:\n %q\natteso:\n %q", c.v, got, c.atteso)
		}
	}
	b, err := json.Marshal(ComponiSorgente(maRisultatoACME(), maUUID(1)))
	if err != nil || !strings.Contains(string(b), `"prodotti":[{`) {
		t.Errorf("il JSON di S9: %v %.300s", err, b)
	}
}
