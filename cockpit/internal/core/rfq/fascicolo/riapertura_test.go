package fascicolo

// L1 — Smistamento F7 (addendum A5.15): il comando U5 come regole pure. Il perimetro delle RFQ, il
// classificatore delle forme legacy F-A … F-L e la provenienza dei componenti e degli archi nati da
// un'evidenza. Le prove L4 del comando (anteprima, applicazione, idempotenza, RFQ congelate e in revisione)
// stanno in riapertura_db_test.go.

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Il perimetro del comando (A5.15.1, Domanda 6 = A): le RFQ aperte con la working non congelata, comprese
// quelle in revisione (una Vn congelata e la bozza aperta), che si dicono a parte; chiuse, unite e congelate
// fuori, con il motivo.
func TestIlPerimetroDelComandoU5(t *testing.T) {
	riga := func(stato db.StatoThread, unita bool, numero int32, statoBom string, congelate int32) db.ListRfqPerLaRiaperturaRow {
		r := db.ListRfqPerLaRiaperturaRow{ThreadID: uuid.New(), Stato: stato, UltimoNumero: numero, UltimoStato: statoBom, NCongelate: congelate}
		if unita {
			r.UnitoIn = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		}
		return r
	}
	for _, c := range []struct {
		nome                   string
		r                      db.ListRfqPerLaRiaperturaRow
		dentro, revisione      bool
		motivo, versioneAttesa string
	}{
		{"aperta senza versioni", riga(db.StatoThreadAPERTA, false, 0, "", 0), true, false, "", ""},
		{"aperta con la V1 in bozza", riga(db.StatoThreadAPERTA, false, 1, "bozza", 0), true, false, "", "V1 bozza"},
		{"in revisione: V1 congelata e V2 in bozza", riga(db.StatoThreadAPERTA, false, 2, "bozza", 1), true, true, "", "V2 bozza"},
		{"congelata: la V1 e' l'ultima", riga(db.StatoThreadAPERTA, false, 1, "congelata", 1), false, false, EsclusaCongelata, "V1 congelata"},
		{"congelata dopo una revisione", riga(db.StatoThreadAPERTA, false, 2, "congelata", 2), false, false, EsclusaCongelata, "V2 congelata"},
		{"chiusa", riga(db.StatoThreadCHIUSA, false, 1, "bozza", 0), false, false, EsclusaChiusa, "V1 bozza"},
		{"chiusa e congelata: conta come chiusa", riga(db.StatoThreadCHIUSA, false, 1, "congelata", 1), false, false, EsclusaChiusa, "V1 congelata"},
		{"unita a un'altra", riga(db.StatoThreadAPERTA, true, 0, "", 0), false, false, EsclusaUnita, ""},
	} {
		p := PerimetroDi(c.r)
		if p.Dentro != c.dentro || p.InRevisione != c.revisione || p.Motivo != c.motivo || p.Versione != c.versioneAttesa {
			t.Errorf("%s: %+v; atteso dentro=%v revisione=%v motivo=%q versione=%q", c.nome, p, c.dentro, c.revisione, c.motivo, c.versioneAttesa)
		}
	}
}

// scenaLegacy e' una RFQ con le righe scritte prima dello Smistamento, come le trova il comando U5.
type scenaLegacy struct {
	utente                 uuid.UUID
	p, q, a, b, k          db.Componente
	aS, aG, aH, aM         uuid.UUID
	docS, docH             uuid.UUID
	nodi                   []db.ListComponenteProposteThreadRow
	archi                  []db.ListRelazioneProposteThreadRow
	dich                   []db.ListDichiarazioniRfqRow
	dati                   DatiRiapertura
	shaS, shaG, shaH, shaM string
}

func (s *scenaLegacy) nodo(all uuid.UUID, sha, file, k, codice string, stato db.StatoProposta, comp *db.Componente, persona bool, nota string) {
	n := db.ComponenteProposta{PropostaID: uuid.New(), AllegatoID: all, Sha256: sha, Chiave: k, Codice: testoP(codice), NomeGrezzo: codice,
		Stato: stato, Evidenza: json.RawMessage(`{}`), CreatoIl: time.Now(), Nota: testoP(nota)}
	if comp != nil {
		n.ComponenteID = uuid.NullUUID{UUID: comp.ComponenteID, Valid: true}
		quando := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
		n.DecisoIl = &quando
	}
	if persona {
		n.DecisoDa = uuid.NullUUID{UUID: s.utente, Valid: true}
	}
	s.nodi = append(s.nodi, db.ListComponenteProposteThreadRow{ComponenteProposta: n, NomeFile: file})
}

func (s *scenaLegacy) arco(all uuid.UUID, file, p, f string, stato db.StatoProposta, persona bool, nota string) {
	r := db.RelazioneProposta{AllegatoID: all, PadreChiave: p, FiglioChiave: f, Qta: 1, Stato: stato, Evidenza: json.RawMessage(`{}`),
		Nota: testoP(nota), CreatoIl: time.Now()}
	if persona {
		r.DecisoDa = uuid.NullUUID{UUID: s.utente, Valid: true}
	}
	s.archi = append(s.archi, db.ListRelazioneProposteThreadRow{RelazioneProposta: r, NomeFile: file})
}

// nuovaScenaLegacy: il caso guida dopo anni di agganci per codice.
//
//	7120001 (finito, P) ha lo STEP strutturale 7120001.stp (S), com'era prima dello Smistamento: la radice #1
//	    (codice 7120001) agganciata a P senza una persona — la radice di prima (F-C), tenuta; #2 (7120010)
//	    agganciato ad A senza una persona (F-A, figlio diretto: nell'autorita'); #3 (7120012) deciso da una
//	    persona come B; #4 (7120011, sotto #2) agganciato a K senza una persona (F-A, guida). Gli archi
//	    #1 → #2 e #2 → #4 chiusi in automatico (F-B); #1 → #3 confermato da una persona.
//	7120001A_1.stp (G), un file qualunque: la radice #1 ha lo STESSO codice del prodotto ed e' agganciata a P
//	    senza una persona. Non e' lo STEP strutturale: e' un aggancio per codice (F-A), non una radice di prima.
//	    Poi le chiusure per sostituzione (F-E), le righe confermate senza chi le ha decise (F-L) e l'arco
//	    #3 → #4 chiuso in automatico fra due nodi che non sono agganci per codice (non e' F-B).
//	7120001A (finito, Q) ha lo STEP strutturale H, la cui radice e' gia' P (P7, F-D).
//	M.stp: la radice legata a P da una persona dall'editor, senza autorizzazione (F-F); il figlio #2 deciso da
//	    una persona come B, e l'arco #1 → #2 chiuso in automatico fra le due decisioni (non e' F-B).
func nuovaScenaLegacy() *scenaLegacy {
	s := &scenaLegacy{utente: uuid.New(), aS: uuid.New(), aG: uuid.New(), aH: uuid.New(), aM: uuid.New(), docS: uuid.New(), docH: uuid.New(),
		shaS: shaDi("a"), shaG: shaDi("b"), shaH: shaDi("c"), shaM: shaDi("d")}
	s.p = componente("7120001", db.TipoComponenteFinito, false)
	s.p.StepStrutturaleID = uuid.NullUUID{UUID: s.docS, Valid: true}
	s.q = componente("7120001A", db.TipoComponenteFinito, false)
	s.q.StepStrutturaleID = uuid.NullUUID{UUID: s.docH, Valid: true}
	s.a = componente("7120010", db.TipoComponenteSottoassieme, false)
	s.a.Origine = db.OrigineComponenteStep
	s.b = componente("7120012", db.TipoComponenteSciolto, false)
	s.b.Origine = db.OrigineComponenteStep
	s.k = componente("7120011", db.TipoComponenteSciolto, false)
	s.k.Origine = db.OrigineComponenteCodiceRilevato
	for _, c := range []*db.Componente{&s.p, &s.q, &s.a, &s.b} {
		if c.Origine == "" {
			c.Origine = db.OrigineComponenteManuale
		}
	}

	s.nodo(s.aS, s.shaS, "7120001.stp", "#1", "7120001", db.StatoPropostaDuplicato, &s.p, false, "")
	s.nodo(s.aS, s.shaS, "7120001.stp", "#2", "7120010", db.StatoPropostaDuplicato, &s.a, false, "")
	s.nodo(s.aS, s.shaS, "7120001.stp", "#3", "7120012", db.StatoPropostaDuplicato, &s.b, true, "")
	s.nodo(s.aS, s.shaS, "7120001.stp", "#4", "7120011", db.StatoPropostaDuplicato, &s.k, false, "")
	s.arco(s.aS, "7120001.stp", "#1", "#2", db.StatoPropostaDuplicato, false, "")
	s.arco(s.aS, "7120001.stp", "#1", "#3", db.StatoPropostaConfermata, true, "")
	s.arco(s.aS, "7120001.stp", "#2", "#4", db.StatoPropostaDuplicato, false, "")

	s.nodo(s.aG, s.shaG, "7120001A_1.stp", "#1", "7120001", db.StatoPropostaDuplicato, &s.p, false, "")
	s.nodo(s.aG, s.shaG, "7120001A_1.stp", "#2", "7120010", db.StatoPropostaAperta, nil, false, "")
	s.nodo(s.aG, s.shaG, "7120001A_1.stp", "#3", "7120012", db.StatoPropostaScartata, nil, false, "superata da 7120001A_2.stp")
	s.nodo(s.aG, s.shaG, "7120001A_1.stp", "#4", "7120012", db.StatoPropostaConfermata, &s.b, false, "")
	s.arco(s.aG, "7120001A_1.stp", "#1", "#2", db.StatoPropostaScartata, false, "superata da 7120001A_2.stp")
	s.arco(s.aG, "7120001A_1.stp", "#1", "#3", db.StatoPropostaAperta, false, "")
	s.arco(s.aG, "7120001A_1.stp", "#2", "#4", db.StatoPropostaConfermata, false, "")
	s.arco(s.aG, "7120001A_1.stp", "#3", "#4", db.StatoPropostaDuplicato, false, "")

	s.nodo(s.aH, s.shaH, "H.stp", "#1", "7120001", db.StatoPropostaDuplicato, &s.p, false, "")
	s.nodo(s.aM, s.shaM, "M.stp", "#1", "7120001", db.StatoPropostaDuplicato, &s.p, true, "")
	s.nodo(s.aM, s.shaM, "M.stp", "#2", "7120012", db.StatoPropostaDuplicato, &s.b, true, "")
	s.arco(s.aM, "M.stp", "#1", "#2", db.StatoPropostaDuplicato, false, "")

	// le dichiarazioni come le legge ListDichiarazioniRfq: la forma di prima, una riga per radice (K1)
	legacy := func(c db.Componente, sha string, all, doc uuid.UUID, comp db.Componente) db.ListDichiarazioniRfqRow {
		r := rigaDich(c, sha, "#1", OrigineStepStrutturale)
		r.AllegatoID, r.DecisoDa = all, uuid.NullUUID{}
		r.RigaComponenteID = uuid.NullUUID{UUID: comp.ComponenteID, Valid: true}
		r.DocumentoID = uuid.NullUUID{UUID: doc, Valid: true}
		return r
	}
	s.dich = []db.ListDichiarazioniRfqRow{legacy(s.p, s.shaS, s.aS, s.docS, s.p), legacy(s.q, s.shaH, s.aH, s.docH, s.p)}

	confermati := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
	s.dati = DatiRiapertura{
		Nodi: s.nodi, Archi: s.archi,
		Componenti: []db.Componente{s.p, s.q, s.a, s.b, s.k},
		Documenti: []db.Documento{
			{DocumentoID: s.docS, ComponenteID: uuid.NullUUID{UUID: s.p.ComponenteID, Valid: true}, NomeFile: "7120001.stp", Sha256: s.shaS, ConfermatoIl: confermati},
			{DocumentoID: s.docH, ComponenteID: uuid.NullUUID{UUID: s.q.ComponenteID, Valid: true}, NomeFile: "H.stp", Sha256: s.shaH, ConfermatoIl: confermati},
			{DocumentoID: uuid.New(), ComponenteID: uuid.NullUUID{UUID: s.a.ComponenteID, Valid: true}, NomeFile: "7120010.pdf", Sha256: shaDi("e"),
				ConfermatoIl: confermati.Add(time.Hour)},
		},
		Relazioni: []db.ComponenteRelazione{
			{PadreID: s.p.ComponenteID, FiglioID: s.a.ComponenteID, Qta: 1, Origine: db.OrigineComponenteStep},
			{PadreID: s.p.ComponenteID, FiglioID: s.b.ComponenteID, Qta: 1, Origine: db.OrigineComponenteStep},
			{PadreID: s.a.ComponenteID, FiglioID: s.k.ComponenteID, Qta: 1, Origine: db.OrigineComponenteManuale},
		},
		Rimozioni: []db.RimozioneProposta{
			{PadreID: s.p.ComponenteID, FiglioID: s.k.ComponenteID, Stato: db.StatoPropostaScartata, Nota: testoP("cambiato lo STEP strutturale")},
			{PadreID: s.p.ComponenteID, FiglioID: s.b.ComponenteID, Stato: db.StatoPropostaScartata, Nota: testoP("scartata"),
				DecisoDa: uuid.NullUUID{UUID: s.utente, Valid: true}},
		},
		Identificativi: []db.IdentificativoThread{
			{Codice: "7120001", Origine: db.OrigineIdentificativoManuale},
			{Codice: "7120001a", Origine: db.OrigineIdentificativoPropostaNomeFile},
		},
		ProposteDocumento: []db.ListProposteDocumentoDellaRfqRow{
			{PropostaID: uuid.New(), Stato: db.StatoPropostaAperta, ComponenteID: uuid.NullUUID{UUID: s.a.ComponenteID, Valid: true},
				Fonte: db.FontePropostaNomeFile, NomeFile: "7120010_disegno.pdf"},
			{PropostaID: uuid.New(), Stato: db.StatoPropostaAperta, Fonte: db.FontePropostaRegolaCliente, RadiceStep: true, NomeFile: "7120001A_1.stp"},
			{PropostaID: uuid.New(), Stato: db.StatoPropostaConfermata, ComponenteID: uuid.NullUUID{UUID: s.b.ComponenteID, Valid: true},
				Fonte: db.FontePropostaNomeFile, NomeFile: "7120012.pdf", DecisoDa: uuid.NullUUID{UUID: s.utente, Valid: true}},
		},
	}
	s.dati.Autorita = CalcolaAutorita(ValutaDichiarazioni(s.dich), soloNodi(s.nodi), soloArchi(s.archi))
	return s
}

// Prova 200 (A5.15.1): il classificatore delle forme legacy, una forma per volta, sulla scena del caso guida.
// La radice di prima dello Smistamento si riconosce dalla relazione (K1: nessun arco entrante nello STEP
// strutturale del finito), non dal codice: la radice dello STEP strutturale di 7120001 si tiene (F-C), mentre la
// radice di un altro file con lo STESSO codice del prodotto, agganciata al prodotto da una lettura, e' un aggancio
// per codice (F-A) e si riapre. Un nodo deciso da una persona, una marcatura, una radice in conflitto P7 (F-D) e
// una radice legata a mano (F-F) non si riaprono mai.
func TestIlClassificatoreDelleFormeLegacy(t *testing.T) {
	s := nuovaScenaLegacy()
	f := ClassificaFormeLegacy(s.dati)

	// F-A: i nodi agganciati per codice senza una persona, con dove stanno
	var fa []string
	for _, n := range f.Agganci {
		fa = append(fa, n.NomeFile+n.Chiave+"→"+n.CodiceComponente+" "+n.Dove)
	}
	sort.Strings(fa)
	attesi := []string{
		"7120001.stp#2→7120010 figlio diretto di 7120001 (STEP autorizzato)",
		"7120001.stp#4→7120011 guida",
		"7120001A_1.stp#1→7120001 guida",
	}
	if !reflect.DeepEqual(fa, attesi) {
		t.Errorf("F-A:\n%s\natteso:\n%s", strings.Join(fa, "\n"), strings.Join(attesi, "\n"))
	}
	for _, n := range f.Agganci {
		if n.AgganciatoIl == nil || n.PropostaID == uuid.Nil || n.ComponenteID == uuid.Nil {
			t.Errorf("F-A senza i dati per la storia: %+v", n)
		}
		if n.Chiave == "#2" && !n.NellAutorita {
			t.Errorf("7120010 e' figlio diretto del file autorizzato di 7120001: %+v", n)
		}
	}
	// F-B: gli archi chiusi in automatico che toccano un F-A; non l'arco deciso da una persona, non la chiusura
	// per sostituzione, non un arco chiuso in automatico senza un estremo fra gli F-A (M.stp #1 → #2, fra due
	// nodi decisi da persone; 7120001A_1.stp #3 → #4, fra due nodi che non sono agganci)
	var fb []string
	for _, x := range f.Archi {
		fb = append(fb, x.NomeFile+" "+x.Padre+">"+x.Figlio+":"+x.Stato)
	}
	sort.Strings(fb)
	if atteso := []string{"7120001.stp #1>#2:duplicato", "7120001.stp #2>#4:duplicato"}; !reflect.DeepEqual(fb, atteso) {
		t.Errorf("F-B = %v, atteso %v", fb, atteso)
	}
	// F-C: la radice dello STEP strutturale, con lo stesso codice del prodotto, tenuta
	if len(f.RadiciTenute) != 1 || f.RadiciTenute[0].NomeFile != "7120001.stp" || f.RadiciTenute[0].Componente != "7120001" || f.RadiciTenute[0].Chiave != "#1" {
		t.Errorf("F-C = %+v, attesa la radice di 7120001.stp", f.RadiciTenute)
	}
	// F-D: la radice dello STEP strutturale di 7120001A e' gia' 7120001 (P7): segnalata, non riaperta
	if len(f.Conflitti) != 1 || !strings.Contains(f.Conflitti[0], "H.stp") || !strings.Contains(f.Conflitti[0], "7120001A") || !strings.Contains(f.Conflitti[0], "P7") {
		t.Errorf("F-D = %v", f.Conflitti)
	}
	// F-E: le chiusure per sostituzione e per un riferimento cambiato, contate (una rimozione decisa da una persona no)
	if f.ChiusureAutomatiche != (ChiusureAutomatiche{Nodi: 1, Archi: 1, Rimozioni: 1}) {
		t.Errorf("F-E = %+v", f.ChiusureAutomatiche)
	}
	// F-F: la radice legata a mano, senza autorizzazione
	if len(f.RadiciAMano) != 1 || f.RadiciAMano[0].NomeFile != "M.stp" || f.RadiciAMano[0].Componente != "7120001" {
		t.Errorf("F-F = %+v", f.RadiciAMano)
	}
	// F-G e F-H: la pre-assegnazione del vecchio «Assegna» e il codice riscritto dalla D16 (una proposta decisa no)
	if !reflect.DeepEqual(f.Preassegnazioni, []string{"7120010_disegno.pdf"}) || !reflect.DeepEqual(f.CodiciRiscritti, []string{"7120001A_1.stp"}) {
		t.Errorf("F-G = %v, F-H = %v", f.Preassegnazioni, f.CodiciRiscritti)
	}
	// F-I: i documenti confermati nello stesso istante
	if len(f.ConfermatiInBlocco) != 1 || !reflect.DeepEqual(f.ConfermatiInBlocco[0].Documenti, []string{"7120001.stp", "H.stp"}) {
		t.Errorf("F-I = %+v", f.ConfermatiInBlocco)
	}
	// F-J: i componenti nati da un'evidenza e mai raggiunti da una decisione. 7120012 no: una persona l'ha deciso
	// nell'autorita'. 7120001 no: ha il suo STEP autorizzato (la forma di prima, valida)
	var fj []string
	for _, c := range f.ComponentiDaRivedere {
		fj = append(fj, c.Componente+": "+strings.Join(c.Etichette, ", "))
	}
	sort.Strings(fj)
	if atteso := []string{
		"7120001A: " + ProvenienzaNomeAllegato,
		"7120010: " + ProvenienzaStepNonAutorizzato,
		"7120011: " + ProvenienzaCodiceTrovato,
	}; !reflect.DeepEqual(fj, atteso) {
		t.Errorf("F-J:\n%s\natteso:\n%s", strings.Join(fj, "\n"), strings.Join(atteso, "\n"))
	}
	// F-K: l'arco 7120001 → 7120010 viene da uno STEP e nessuna persona l'ha deciso nell'autorita'; 7120001 →
	// 7120012 si' (confermato da una persona dal file autorizzato); 7120010 → 7120011 e' manuale
	if len(f.ArchiDaRivedere) != 1 || f.ArchiDaRivedere[0] != (ArcoDaRivedere{Padre: "7120001", Figlio: "7120010"}) {
		t.Errorf("F-K = %+v", f.ArchiDaRivedere)
	}
	// F-L: un nodo e un arco confermati senza chi li ha decisi; il P7 e' gia' F-D e non si ripete
	if len(f.Anomalie) != 2 || !strings.Contains(strings.Join(f.Anomalie, "|"), "il nodo #4 di 7120001A_1.stp è confermato senza chi l'ha deciso") ||
		!strings.Contains(strings.Join(f.Anomalie, "|"), "l'arco #2 → #4 di 7120001A_1.stp è confermato senza chi l'ha deciso") {
		t.Errorf("F-L = %v", f.Anomalie)
	}
	// l'effetto nel gate: un figlio diretto da confermare in un file autorizzato
	if f.Effetto.FigliDaConfermare != 1 || f.Effetto.FileAutorizzati != 1 ||
		f.Effetto.Frase != "il gate si fermerà su 1 figlio diretto da confermare in 1 file autorizzato" {
		t.Errorf("effetto = %+v", f.Effetto)
	}
	if !f.DaRiaprire() || f.Vuote() {
		t.Errorf("la scena ha righe da riaprire")
	}
}

// Il classificatore non tocca mai una marcatura (A5.4.2): una riga marcata, anche se sembra un aggancio
// automatico (duplicato senza chi l'ha deciso: una marcatura incoerente, F-L dal predicato), non e' F-A, e un
// arco automatico che la tocca non e' F-B. Una RFQ senza righe di prima non ha niente da riaprire.
func TestLeMarcatureNonSonoFormeLegacy(t *testing.T) {
	s := nuovaScenaLegacy()
	for i := range s.dati.Nodi {
		if s.dati.Nodi[i].NomeFile == "7120001A_1.stp" && s.dati.Nodi[i].ComponenteProposta.Chiave == "#1" {
			s.dati.Nodi[i].ComponenteProposta.Evidenza = json.RawMessage(`{"strutturale": {"v": 1, "ruolo": "radice", "sospesa": {"motivo": "commerciale"}}}`)
		}
	}
	f := ClassificaFormeLegacy(s.dati)
	for _, n := range f.Agganci {
		if n.NomeFile == "7120001A_1.stp" {
			t.Errorf("una riga marcata e' diventata F-A: %+v", n)
		}
	}
	if len(f.Agganci) != 2 {
		t.Errorf("F-A = %d, attesi i 2 di 7120001.stp", len(f.Agganci))
	}

	vuota := ClassificaFormeLegacy(DatiRiapertura{})
	if vuota.DaRiaprire() || !vuota.Vuote() || vuota.Effetto.FigliDaConfermare != 0 || !strings.Contains(vuota.Effetto.Frase, "il gate non cambia") {
		t.Errorf("una RFQ senza righe: %+v", vuota)
	}
}

// Prova 199 (A5.15.4): la provenienza di un componente nato da un'evidenza. Le quattro etichette, e il segno
// che sparisce quando una decisione raggiunge il componente: una riga nell'autorita' decisa da una persona, o
// un'autorizzazione valida sua. Un aggancio automatico nell'autorita' non e' una decisione e non basta.
func TestLaProvenienzaDiUnComponenteNatoDaUnEvidenza(t *testing.T) {
	p := componente("7120001", db.TipoComponenteFinito, false)
	p.Origine = db.OrigineComponenteManuale
	sha := shaDi("a")
	all, nodi, archi := righeAutorita(sha)
	aut := CalcolaAutorita(ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{rigaDich(p, sha, "#1", OrigineSmistamento)}), nodi, archi)
	nessuna := CalcolaAutorita(ValutaDichiarazioni(nil), nodi, archi)
	persona := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	riga := func(k string, c db.Componente, stato db.StatoProposta, decisa bool, radice bool) RigaDelComponente {
		n := db.ComponenteProposta{PropostaID: uuid.New(), AllegatoID: all, Sha256: sha, Chiave: k, Stato: stato,
			ComponenteID: uuid.NullUUID{UUID: c.ComponenteID, Valid: true}, Evidenza: json.RawMessage(`{}`)}
		if decisa {
			n.DecisoDa = persona
		}
		return RigaDelComponente{Proposta: n, Radice: radice}
	}
	nuovo := func(codice string, tipo db.TipoComponente, origine db.OrigineComponente) db.Componente {
		c := componente(codice, tipo, false)
		c.Origine = origine
		return c
	}

	a := nuovo("7120010", db.TipoComponenteSottoassieme, db.OrigineComponenteCodiceRilevato)
	if got := Provenienza(ProvenienzaDi{Componente: a}, aut); !reflect.DeepEqual(got, []string{ProvenienzaCodiceTrovato}) {
		t.Errorf("codice rilevato: %v", got)
	}
	// un aggancio automatico nell'autorita' (#2 e' figlio diretto) non e' una decisione: il segno resta
	if got := Provenienza(ProvenienzaDi{Componente: a, Righe: []RigaDelComponente{riga("#2", a, db.StatoPropostaDuplicato, false, false)}}, aut); len(got) != 1 {
		t.Errorf("con un aggancio automatico nell'autorita': %v", got)
	}
	// una persona l'ha deciso nell'autorita': il segno sparisce
	if got := Provenienza(ProvenienzaDi{Componente: a, Righe: []RigaDelComponente{riga("#2", a, db.StatoPropostaDuplicato, true, false)}}, aut); got != nil {
		t.Errorf("deciso da una persona nell'autorita': %v", got)
	}
	// deciso da una persona ma sulla guida (#4 e' un nipote): resta
	if got := Provenienza(ProvenienzaDi{Componente: a, Righe: []RigaDelComponente{riga("#4", a, db.StatoPropostaConfermata, true, false)}}, aut); len(got) != 1 {
		t.Errorf("deciso da una persona sulla guida: %v", got)
	}
	// senza nessuna autorizzazione, nessuna riga e' nell'autorita'
	if got := Provenienza(ProvenienzaDi{Componente: a, Righe: []RigaDelComponente{riga("#2", a, db.StatoPropostaDuplicato, true, false)}}, nessuna); len(got) != 1 {
		t.Errorf("senza autorizzazioni: %v", got)
	}

	s := nuovo("7120012", db.TipoComponenteSciolto, db.OrigineComponenteStep)
	if got := Provenienza(ProvenienzaDi{Componente: s, Righe: []RigaDelComponente{riga("#4", s, db.StatoPropostaConfermata, true, false)}}, aut); !reflect.DeepEqual(got, []string{ProvenienzaStepNonAutorizzato}) {
		t.Errorf("accettato da uno STEP non autorizzato: %v", got)
	}

	f := nuovo("7120001A", db.TipoComponenteFinito, db.OrigineComponenteManuale)
	dalNome := &db.IdentificativoThread{Codice: "7120001A", Origine: db.OrigineIdentificativoPropostaNomeFile}
	if got := Provenienza(ProvenienzaDi{Componente: f, Identificativo: dalNome}, aut); !reflect.DeepEqual(got, []string{ProvenienzaNomeAllegato}) {
		t.Errorf("prodotto da un codice visto nel nome: %v", got)
	}
	if got := Provenienza(ProvenienzaDi{Componente: f, Identificativo: &db.IdentificativoThread{Codice: "7120001A", Origine: db.OrigineIdentificativoManuale}}, aut); got != nil {
		t.Errorf("prodotto scritto da una persona: %v", got)
	}
	// l'etichetta del nome allegato e' dei soli prodotti (finiti): un componente sciolto o un sottoassieme con lo
	// stesso codice di un identificativo visto nel nome di un allegato non la prende
	for _, tipo := range []db.TipoComponente{db.TipoComponenteSciolto, db.TipoComponenteSottoassieme} {
		nf := nuovo("7120011", tipo, db.OrigineComponenteManuale)
		if got := Provenienza(ProvenienzaDi{Componente: nf, Identificativo: &db.IdentificativoThread{Codice: "7120011",
			Origine: db.OrigineIdentificativoPropostaNomeFile}}, aut); got != nil {
			t.Errorf("un %s con il codice visto nel nome di un allegato: %v, attesa nessuna etichetta", tipo, got)
		}
	}
	// «È il prodotto» dall'editor: la radice di un file legata da una persona, senza autorizzazione (F-F)
	altro := shaDi("f")
	radice := riga("#1", f, db.StatoPropostaDuplicato, true, true)
	radice.Proposta.Sha256, radice.Proposta.AllegatoID = altro, uuid.New() // un altro file
	if got := Provenienza(ProvenienzaDi{Componente: f, Righe: []RigaDelComponente{radice}}, aut); !reflect.DeepEqual(got, []string{ProvenienzaRadiceAMano}) {
		t.Errorf("radice legata a mano: %v", got)
	}
	if got := Provenienza(ProvenienzaDi{Componente: f, StepSha: altro, Righe: []RigaDelComponente{radice}}, aut); got != nil {
		t.Errorf("la radice del suo STEP strutturale non e' legata a mano: %v", got)
	}
	marcata := radice
	marcata.Proposta.Evidenza = json.RawMessage(`{"strutturale": {"v": 1, "ruolo": "radice"}}`)
	if got := Provenienza(ProvenienzaDi{Componente: f, Righe: []RigaDelComponente{marcata}}, aut); got != nil {
		t.Errorf("una radice marcata e' un'autorizzazione, non una radice legata a mano: %v", got)
	}
	// tutte insieme, nell'ordine delle etichette
	tutto := nuovo("7120001A", db.TipoComponenteFinito, db.OrigineComponenteCodiceRilevato)
	if got := Provenienza(ProvenienzaDi{Componente: tutto, Identificativo: dalNome, Righe: []RigaDelComponente{riga("#1", tutto, db.StatoPropostaDuplicato, true, true)}}, nessuna); !reflect.DeepEqual(got,
		[]string{ProvenienzaCodiceTrovato, ProvenienzaNomeAllegato, ProvenienzaRadiceAMano}) {
		t.Errorf("tutte le etichette: %v", got)
	}
	// un'autorizzazione valida sua: nessuna etichetta, qualunque sia l'origine
	pr := nuovo("7120001", db.TipoComponenteFinito, db.OrigineComponenteCodiceRilevato)
	pr.ComponenteID = p.ComponenteID
	if got := Provenienza(ProvenienzaDi{Componente: pr, Identificativo: dalNome}, aut); got != nil {
		t.Errorf("con il suo STEP autorizzato: %v", got)
	}

	// gli archi (F-K): accettati da uno STEP e mai decisi da una persona nell'autorita' del padre
	b := nuovo("7120010", db.TipoComponenteSottoassieme, db.OrigineComponenteStep)
	nodi[1].ComponenteID, nodi[1].DecisoDa, nodi[1].Stato = uuid.NullUUID{UUID: b.ComponenteID, Valid: true}, persona, db.StatoPropostaConfermata
	archi[0].DecisoDa, archi[0].Stato = persona, db.StatoPropostaConfermata // #1 → #2, dalla sorgente
	decisi := ArchiDecisiNellAutorita(CalcolaAutorita(aut.Dichiarazioni, nodi, archi), nodi, archi)
	for _, c := range []struct {
		nome   string
		r      db.ComponenteRelazione
		atteso bool
	}{
		{"deciso da una persona dal file autorizzato", db.ComponenteRelazione{PadreID: p.ComponenteID, FiglioID: b.ComponenteID, Origine: db.OrigineComponenteStep}, false},
		{"da uno STEP, mai deciso nell'autorita'", db.ComponenteRelazione{PadreID: p.ComponenteID, FiglioID: uuid.New(), Origine: db.OrigineComponenteStep}, true},
		{"messo a mano", db.ComponenteRelazione{PadreID: p.ComponenteID, FiglioID: uuid.New(), Origine: db.OrigineComponenteManuale}, false},
	} {
		if got := ArcoDaRivedereDi(c.r, decisi); got != c.atteso {
			t.Errorf("%s: %v, atteso %v", c.nome, got, c.atteso)
		}
	}
	// lo stesso arco deciso in automatico (la tenuta dei conti di prima) non basta
	archi[0].DecisoDa = uuid.NullUUID{}
	if decisi := ArchiDecisiNellAutorita(CalcolaAutorita(aut.Dichiarazioni, nodi, archi), nodi, archi); !ArcoDaRivedereDi(
		db.ComponenteRelazione{PadreID: p.ComponenteID, FiglioID: b.ComponenteID, Origine: db.OrigineComponenteStep}, decisi) {
		t.Errorf("un arco chiuso da un automatismo non e' una decisione")
	}
}
