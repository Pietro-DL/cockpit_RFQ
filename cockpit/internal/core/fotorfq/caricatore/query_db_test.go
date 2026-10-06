//go:build integrazione

// L4 — le letture del caricatore, record per record (A1c-L4S-10, riscritta per il contratto di A1c, §3 e §3.4; piano A,
// 6.5 e 6.7.3; contratto §7, famiglia B1): i tre gesti, i documenti con le provenienze e i fatti dei documenti
// (T-04), le righe legacy con la marcatura e il segno dell'albero, le rimozioni aperte, le viste v_step_prodotto e
// v_fascicolo, le deroghe, i fabbisogni effettivi del cliente, la versione della BOM e l'ultima congelata, il lavoro
// pendente; il triage e i candidati dell'agente esclusi; nessuna password_hash e nessuna regola del cliente, né
// nella fotografia né nelle colonne delle query nuove.
//
// La scena è inventata (ACME): nessun dato reale, il repository è pubblico.

package caricatore

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

func TestLeLettureDelCaricatore(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	ctx := context.Background()
	f, err := Carica(ctx, p, Richiesta{Thread: []uuid.UUID{s.thread, s.thread2}, Messaggi: []uuid.UUID{s.fuori}})
	if err != nil {
		t.Fatal(err)
	}
	th := threadDi(t, f, s.thread)

	// il thread
	if th.ClienteID != s.cliente || th.Stato != "APERTA" || th.Oggetto == nil || *th.Oggetto != "RFQ ACME26-030" ||
		th.Riferimento == nil || *th.Riferimento != "ACME26-030" || th.CreatoDa == nil || *th.CreatoDa != s.operatore || th.UnitoIn != nil {
		t.Errorf("thread: %+v", th)
	}

	// il gesto 1, uno per messaggio, fuori dal record del messaggio (I-2)
	agganci := map[uuid.UUID]fotorfq.AggancioMessaggio{}
	for _, a := range th.Agganci {
		agganci[a.MessaggioID] = a
	}
	if a := agganci[s.m1]; a.Aggancio != "operatore" || a.AgganciatoDa == nil || *a.AgganciatoDa != s.operatore || a.AgganciatoIl == nil {
		t.Errorf("gesto 1 del primo messaggio: %+v", a)
	}
	if a := agganci[s.m2]; a.Aggancio != "auto_conversazione" || a.AgganciatoDa != nil || a.AgganciatoIl != nil {
		t.Errorf("aggancio automatico: %+v", a)
	}
	if len(th.Messaggi) != 2 || len(th.Agganci) != 2 {
		t.Errorf("messaggi %d, agganci %d", len(th.Messaggi), len(th.Agganci))
	}

	// il gesto 2
	confermati := 0
	for _, x := range th.Identificativi {
		if x.ConfermatoDa != nil {
			confermati++
			if x.Codice != "7120001" || *x.ConfermatoDa != s.operatore {
				t.Errorf("identificativo confermato: %+v", x)
			}
		} else if x.Codice != "7120009" || x.Confidenza == nil || *x.Confidenza != 40 {
			t.Errorf("identificativo proposto: %+v", x)
		}
	}
	if confermati != 1 || len(th.Identificativi) != 2 {
		t.Errorf("identificativi: %+v", th.Identificativi)
	}

	// i componenti con il gesto 3, le relazioni
	componenti := map[uuid.UUID]fotorfq.Componente{}
	for _, c := range th.Componenti {
		componenti[c.ID] = c
	}
	if c := componenti[s.finito]; c.Tipo != "finito" || c.StepStrutturaleID == nil || *c.StepStrutturaleID != s.docStep || c.ConfermatoDa != s.operatore {
		t.Errorf("finito: %+v", c)
	}
	if c := componenti[s.sciolto]; c.Rev == nil || *c.Rev != "1" || c.Descrizione == nil || c.StepStrutturaleID != nil {
		t.Errorf("sciolto: %+v", c)
	}
	if len(th.Relazioni) != 1 || th.Relazioni[0].PadreID != s.finito || th.Relazioni[0].Qta != 2 ||
		th.Relazioni[0].Posizione == nil || *th.Relazioni[0].Posizione != "10" {
		t.Errorf("relazioni: %+v", th.Relazioni)
	}

	// i documenti, con le provenienze e i fatti dei documenti
	documenti := map[uuid.UUID]fotorfq.DocumentoConfermato{}
	for _, d := range th.Documenti {
		documenti[d.ID] = d
	}
	if d := documenti[s.docStep]; d.Tipo != "cad_3d" || d.Estensione != "stp" || d.Sha256 != shaStep || !slices.Equal(d.Allegati, []uuid.UUID{s.aStep}) {
		t.Errorf("documento STEP: %+v", d)
	}
	if d := documenti[s.docPDF]; d.StatoNas != "scritto" || d.Rev == nil || *d.Rev != "1" || d.ComponenteID == nil || *d.ComponenteID != s.sciolto {
		t.Errorf("documento 2D: %+v", d)
	}
	if d := documenti[s.docSoloDoc]; d.ComponenteID != nil || len(d.Allegati) != 0 {
		t.Errorf("documento del thread: %+v", d)
	}
	if _, ok := th.Fatti[shaSoloDoc]; !ok {
		t.Error("i fatti del documento senza allegato nel thread mancano (T-04)")
	}
	for sha, x := range th.Fatti {
		if d, err := fotorfq.ImprontaPayload(x.Payload); err != nil || d != x.Digest || x.Sha256 != sha {
			t.Errorf("fatti di %s: digest %s (%v)", sha, x.Digest, err)
		}
	}

	// le righe legacy, con la marcatura (gesto 3) e il segno dell'albero
	righe := map[uuid.UUID]fotorfq.RigaComponenteProposta{}
	for _, r := range th.RigheComponenteProposta {
		righe[r.ID] = r
	}
	radice := righe[s.rigaRadice]
	if m := radice.Marcatura; m == nil || m.Ruolo != "radice" || m.Versione != 1 || m.DocumentoID == nil || *m.DocumentoID != s.docStep ||
		m.ComponenteID == nil || *m.ComponenteID != s.finito || m.DichiaratoDa == nil || *m.DichiaratoDa != s.operatore ||
		m.DichiaratoIl != "2026-10-06T08:00:00Z" || m.Sospesa {
		t.Errorf("marcatura della radice: %+v", m)
	}
	if radice.NomeFile != "ACME-030P7120001.stp" || radice.IDGrezzo != "ACME-030P7120001" || radice.Albero != nil || radice.Stato != "confermata" {
		t.Errorf("riga della radice: %+v", radice)
	}
	figlio := righe[s.rigaFiglio]
	if figlio.OrigineCodice == nil || *figlio.OrigineCodice != "operatore" || figlio.Codice == nil || *figlio.Codice != "7120002" ||
		figlio.TipoProposto == nil || *figlio.TipoProposto != "sciolto" || figlio.Marcatura != nil {
		t.Errorf("riga del figlio: %+v", figlio)
	}
	if a := figlio.Albero; a == nil || a.Da != s.operatore || a.Firma != "f-acme" || a.Nodo != "cod:7120002" {
		t.Errorf("segno dell'albero sul nodo: %+v", a)
	}
	if len(th.RigheRelazioneProposta) != 1 {
		t.Fatalf("righe di arco: %+v", th.RigheRelazioneProposta)
	}
	arco := th.RigheRelazioneProposta[0]
	if a := arco.Albero; arco.Qta != 2 || arco.PadreChiave != "#1" || a == nil || a.Padre == nil || *a.Padre != s.finito ||
		a.Figlio == nil || *a.Figlio != s.sciolto {
		t.Errorf("riga dell'arco: %+v", arco)
	}

	// le rimozioni aperte, le viste, le deroghe, i fabbisogni effettivi
	if len(th.RimozioniAperte) != 1 || th.RimozioniAperte[0] != (fotorfq.RimozioneAperta{StepDocumentoID: s.docStep, PadreID: s.finito,
		FiglioID: s.sciolto, QtaWorking: 2}) {
		t.Errorf("rimozioni aperte: %+v", th.RimozioniAperte)
	}
	if len(th.StepProdotto) != 1 || th.StepProdotto[0].ComponenteID != s.finito || th.StepProdotto[0].StepStrutturaleID == nil ||
		*th.StepProdotto[0].StepStrutturaleID != s.docStep || th.StepProdotto[0].Esito == "" {
		t.Errorf("v_step_prodotto: %+v", th.StepProdotto)
	}
	perComponente := map[uuid.UUID]int{}
	for _, r := range th.Fascicolo {
		perComponente[r.ComponenteID]++
		if r.Esito == "" || r.TipoDocumento == "" {
			t.Errorf("riga di v_fascicolo: %+v", r)
		}
	}
	if perComponente[s.finito] == 0 || perComponente[s.sciolto] == 0 {
		t.Errorf("v_fascicolo senza le righe dei due componenti: %+v", th.Fascicolo)
	}
	if len(th.Deroghe) != 1 || th.Deroghe[0].ID != s.deroga || th.Deroghe[0].Tipo != "sviluppo_dxf" || th.Deroghe[0].UtenteID != s.operatore {
		t.Errorf("deroghe: %+v", th.Deroghe)
	}
	propri, finitiDefault := 0, 0
	for _, r := range th.Fabbisogni {
		if r.Proprio {
			propri++
			if r.TipoComponente != "sciolto" || r.TipoDocumento != "sviluppo_dxf" || r.FonteAttesa == nil || *r.FonteAttesa != "cliente" {
				t.Errorf("fabbisogno del cliente: %+v", r)
			}
		}
		if r.TipoComponente == "finito" && !r.Proprio {
			finitiDefault++
		}
		if r.TipoComponente == "sciolto" && !r.Proprio {
			t.Errorf("per lo sciolto la riga del cliente sostituisce i default: %+v", r)
		}
	}
	if propri != 1 || finitiDefault == 0 {
		t.Errorf("fabbisogni effettivi: %+v", th.Fabbisogni)
	}

	// le proposte, con l'associazione manuale e i dettagli
	proposte := map[uuid.UUID]fotorfq.PropostaAttuale{}
	for _, x := range th.Proposte {
		proposte[x.ID] = x
	}
	if x := proposte[s.proposta2D]; x.Stato != "confermata" || x.DecisoDa == nil || len(x.Dettagli) == 0 || !strings.Contains(string(x.Dettagli), "destinazione") {
		t.Errorf("proposta confermata: %+v", x)
	}
	if x := proposte[s.propostaAssegna]; x.Stato != "aperta" || x.ComponenteID == nil || *x.ComponenteID != s.sciolto {
		t.Errorf("proposta assegnata: %+v", x)
	}

	// la versione della BOM: l'ultima (la V2 in bozza) e l'ultima congelata (la V1, con chi e quando)
	if v := th.VersioneBOM; v == nil || v.ID != s.v2 || v.Numero != 2 || v.Stato != "bozza" || v.CongelataDa != nil {
		t.Errorf("versione della BOM: %+v", v)
	}
	if v := th.UltimaCongelata; v == nil || v.ID != s.v1 || v.Stato != "congelata" || v.Contesto != "preventivo" ||
		v.CongelataDa == nil || *v.CongelataDa != s.operatore || v.CongelataIl == nil {
		t.Errorf("ultima congelata: %+v", v)
	}
	th2 := threadDi(t, f, s.thread2)
	if th2.VersioneBOM != nil || th2.UltimaCongelata != nil || len(th2.Agganci) != 1 || th2.Agganci[0].AgganciatoDa == nil {
		t.Errorf("seconda RFQ: %+v", th2)
	}

	// il lavoro pendente: lo stesso allegato da due job, una volta sola
	if !slices.Equal(th.InAttesa, []uuid.UUID{s.aSenzaFatti}) {
		t.Errorf("allegati in attesa: %v", th.InAttesa)
	}

	// il triage e i candidati dell'agente esclusi
	if len(th.Triage) != 1 || th.Triage[0].Esito != "nuova_rfq" || !slices.Equal(th.Triage[0].Identificativi, []string{"7120001"}) {
		t.Errorf("triage: %+v", th.Triage)
	}
	if len(th.CandidatiCodice) != 1 || th.CandidatiCodice[0].Codice != "7120001" || th.CandidatiCodice[0].Origine == "agente" {
		t.Errorf("candidati di codice: %+v", th.CandidatiCodice)
	}

	// il messaggio senza RFQ, i clienti, gli utenti
	if fu := f.FuoriRFQ[0]; fu.Aggancio.MessaggioID != s.fuori || len(fu.Allegati) != 1 || fu.Allegati[0].ID != s.aFuori {
		t.Errorf("messaggio senza RFQ: %+v", fu)
	}
	if len(f.Clienti) != 1 || f.Clienti[0] != (fotorfq.Cliente{ID: s.cliente, RagioneSociale: "ACME S.p.A."}) {
		t.Errorf("clienti: %+v", f.Clienti)
	}
	sigle := map[string]uuid.UUID{}
	for _, u := range f.Utenti {
		sigle[u.Sigla] = u.ID
	}
	if sigle["AC"] != s.operatore || sigle["AB"] != s.secondo {
		t.Errorf("utenti: %+v", f.Utenti)
	}

	// nessuna password e nessuna regola del cliente, né nella fotografia né nelle colonne delle query nuove
	tutto := inJSON(t, f)
	for _, segreto := range []string{segretoPassword, marcatoreRegole} {
		if strings.Contains(tutto, segreto) {
			t.Errorf("la fotografia porta %q", segreto)
		}
	}
	for _, c := range []struct {
		riga  any
		campi []string
	}{
		{db.ListSigleUtentiRow{}, []string{"UtenteID", "Sigla"}},
		{db.ListClientiDellaFotografiaRow{}, []string{"ClienteID", "RagioneSociale"}},
	} {
		ty := reflect.TypeOf(c.riga)
		var campi []string
		for i := 0; i < ty.NumField(); i++ {
			campi = append(campi, ty.Field(i).Name)
		}
		if !slices.Equal(campi, c.campi) {
			t.Errorf("%s: colonne %v, attese %v", ty.Name(), campi, c.campi)
		}
	}

	// i riferimenti della fotografia si risolvono
	if d := fotorfq.ValidaFotografia(f); len(d) != 0 {
		t.Errorf("diagnostiche di contratto sulla scena: %+v", d)
	}
}
