package calibrazione

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// L1 — Smistamento M3 (A5.14.6, A5.16.6): la retro ordina come il pannello, e la stampa non dice una
// precisione sotto il campione. RFQ finte.

func riga(m, scelto, cand uuid.UUID, regola string, punti int32, evidenza string) db.RetroPostaRow {
	r := db.RetroPostaRow{MessaggioID: m, Scelto: uuid.NullUUID{UUID: scelto, Valid: true}, Regola: regola, Punteggio: punti, Evidenza: evidenza}
	if cand != uuid.Nil {
		r.Candidato = uuid.NullUUID{UUID: cand, Valid: true}
	}
	return r
}

// 236 (parte posta, la retro) — i candidati di un messaggio già deciso si ordinano come li ordina il
// pannello di oggi, non come li ordina il punteggio scritto: un R1 delle regole di prima (95) è il solo
// ConversationID e sta sotto un R3 dello stesso buyer (72). Un messaggio senza candidati si conta a parte.
func TestLaRetroOrdinaComeIlPannello(t *testing.T) {
	t1, t2 := uuid.New(), uuid.New()
	m1, m2, m3 := uuid.New(), uuid.New(), uuid.New()
	righe := []db.RetroPostaRow{
		// m1: scelto T1; nell'ordine della query T2 (95) viene prima, nel pannello T1 (medio-forte) sì
		riga(m1, t1, t2, "R1_conversazione", 95, "stessa conversazione"),
		riga(m1, t1, t1, "R3_codice", 72, "codice 7120001 dello stesso buyer"),
		// m2: scelto T1, ma il solo candidato era T2 con R0: primo sbagliato, e T1 fuori lista
		riga(m2, t1, t2, "R0_reply", 98, "risponde a una mail della RFQ"),
		// m3: agganciato senza nessun candidato
		riga(m3, t1, uuid.Nil, "", 0, ""),
	}
	out, senza := Retro(righe)
	if senza != 1 {
		t.Errorf("senza candidati: %d, atteso 1", senza)
	}
	cella := func(per, livello, tipo, fascia string) RigaRetro {
		t.Helper()
		for _, r := range out {
			if r.Per == per && r.Livello == livello && r.Tipo == tipo && r.Fascia == fascia {
				return r
			}
		}
		t.Fatalf("manca la cella %s/%s/%s/%s in %+v", per, livello, tipo, fascia, out)
		return RigaRetro{}
	}
	if c := cella(PerTotale, "", "", ""); c.Messaggi != 2 || c.AlPrimo != 1 || c.FuoriLista != 1 || c.DiPrima != 0 {
		t.Errorf("totale: %+v", c)
	}
	if c := cella(PerTipo, "", "R3CodiceBuyer", ""); c.Messaggi != 1 || c.AlPrimo != 1 {
		t.Errorf("il primo di m1 deve essere il R3 del buyer, e giusto: %+v", c)
	}
	if c := cella(PerLivello, "molto_forte", "", ""); c.Messaggi != 1 || c.AlPrimo != 0 || c.FuoriLista != 1 {
		t.Errorf("m2, molto forte e sbagliato: %+v", c)
	}
	if c := cella(PerFascia, "", "", "70-89"); c.Messaggi != 1 {
		t.Errorf("fascia del primo di m1: %+v", c)
	}
	cella(PerTipoFascia, "", "R0InReplyTo", "90-100")
	// il totale viene per primo nella stampa
	if out[0].Per != PerTotale {
		t.Errorf("il totale non è la prima riga: %+v", out[0])
	}
	// con la sola riga delle regole di prima, il primo è letto verso il basso e contato come «di prima»
	out, _ = Retro([]db.RetroPostaRow{riga(m1, t1, t1, "R1_conversazione", 95, "stessa conversazione")})
	if c := out[0]; c.Per != PerTotale || c.DiPrima != 1 || c.AlPrimo != 1 {
		t.Errorf("riga di prima: %+v", c)
	}
	if c := cella2(out, PerTipo); c.Tipo != "R1Solo" {
		t.Errorf("R1 a 95 si legge come il solo ConversationID: %+v", c)
	}
}

func cella2(out []RigaRetro, per string) RigaRetro {
	for _, r := range out {
		if r.Per == per {
			return r
		}
	}
	return RigaRetro{}
}

// 236 (parte posta, la stampa) — prima riga host e database, niente password; sotto 20 decisioni la cella
// dice «campione insufficiente» e non un numero, e l'MRR «–»; da 20 in su la precisione con tre decimali;
// nessun «%». Le due precisioni hanno il loro denominatore: la precision@1 di r1_mail.md §7.3 sui soli
// agganci, il «primo giusto» su tutte le decisioni con una lista (ignora e RFQ nuova sono un primo
// sbagliato), e ciascuna ha il suo campione.
func TestLaStampaNonDiceUnaPrecisioneSottoIlCampione(t *testing.T) {
	r := Rapporto{Host: "127.0.0.1:5433", Database: "cockpit_prova", SolaLettura: true, Posta: Posta{
		Misure: []RigaPosta{
			{Regole: "aggancio-2", Per: PerTotale, Decisioni: 3, AlPrimo: 2, NeiPrimi3: 3, Agganci: 3, AgganciAlPrimo: 2, MRR: 0.833},
			// 30 decisioni: 25 agganci (20 al primo), 2 RFQ nuove e 3 ignorati
			{Regole: "aggancio-2", Per: PerLivello, Livello: "molto_forte", Decisioni: 30, AlPrimo: 20, NeiPrimi3: 29, Agganci: 25,
				AgganciAlPrimo: 20, NuoveRFQ: 2, Ignorati: 3, MRR: 0.75},
			// 22 decisioni ma solo 12 agganci: il «primo giusto» si dice, la precision@1 no
			{Regole: "aggancio-2", Per: PerTipo, Tipo: "R3CodiceBuyer", Decisioni: 22, AlPrimo: 9, NeiPrimi3: 12, Agganci: 12,
				AgganciAlPrimo: 9, Ignorati: 10, MRR: 0.5},
		},
		Retro:               []RigaRetro{{Per: PerTotale, Messaggi: 25, AlPrimo: 5}},
		RetroSenzaCandidati: 4,
	}}
	var b bytes.Buffer
	if err := r.Scrivi(&b); err != nil {
		t.Fatal(err)
	}
	testo := b.String()
	righe := strings.Split(testo, "\n")
	if righe[0] != "calibrazione: database cockpit_prova su 127.0.0.1:5433" {
		t.Errorf("prima riga: %q", righe[0])
	}
	if !strings.Contains(righe[1], "sola lettura: sì") {
		t.Errorf("seconda riga: %q", righe[1])
	}
	var totale, livello, tipo, retro string
	for _, l := range righe {
		switch {
		case strings.HasPrefix(l, "aggancio-2") && strings.Contains(l, " totale "):
			totale = l
		case strings.HasPrefix(l, "aggancio-2") && strings.Contains(l, "molto_forte"):
			livello = l
		case strings.HasPrefix(l, "aggancio-2") && strings.Contains(l, "R3CodiceBuyer"):
			tipo = l
		case strings.HasPrefix(l, "totale "):
			retro = l
		}
	}
	if strings.Count(totale, FraseCampioneInsufficiente) != 2 || strings.Contains(totale, "0,667") {
		t.Errorf("3 decisioni: nessuna delle due precisioni si dice: %q", totale)
	}
	if strings.Contains(totale, "0,833") || !strings.Contains(totale, " "+SenzaCampione+" ") {
		t.Errorf("3 decisioni: l'MRR non si dice: %q", totale)
	}
	if !strings.Contains(livello, "0,800") || !strings.Contains(livello, "0,667") || strings.Contains(livello, FraseCampioneInsufficiente) {
		t.Errorf("30 decisioni, 25 agganci e 20 al primo: precision@1 0,800, primo giusto 0,667: %q", livello)
	}
	if !strings.Contains(livello, "0,750") {
		t.Errorf("30 decisioni: l'MRR si dice: %q", livello)
	}
	if strings.Count(tipo, FraseCampioneInsufficiente) != 1 || !strings.Contains(tipo, "0,409") || !strings.Contains(tipo, "0,500") {
		t.Errorf("12 agganci su 22 decisioni: precision@1 insufficiente, primo giusto 0,409, MRR 0,500: %q", tipo)
	}
	if !strings.Contains(retro, "0,200") {
		t.Errorf("retro, 25 messaggi e 5 giusti: %q", retro)
	}
	// «primo giusto» ha un solo significato nella stampa, la frequenza su tutte le decisioni delle
	// fotografie: il conteggio della retro è della stessa grandezza di «agganciati al primo» e si chiama così
	intestazioneRetro := false
	for _, l := range righe {
		if strings.HasPrefix(l, "per ") {
			intestazioneRetro = true
			if strings.Contains(l, "primo giusto") || !strings.Contains(l, "  agganciati al primo  ") {
				t.Errorf("intestazione della retro: %q", l)
			}
		}
	}
	if !intestazioneRetro {
		t.Error("manca l'intestazione della retro")
	}
	if !strings.Contains(testo, "senza nessun candidato: 4") {
		t.Error("manca il conto dei messaggi senza candidati")
	}
	if strings.Contains(testo, "%") {
		t.Error("la stampa contiene «%»")
	}
	// i valori della cella ci sono anche sotto il campione, per chi li vuole guardare
	if p, ok := r.Posta.Misure[0].Precisione(); ok || p < 0.666 || p > 0.667 {
		t.Errorf("Precisione di 2 su 3: %v, sufficiente %v", p, ok)
	}
	// la definizione: precision@1 sugli agganci, primo giusto su tutte le decisioni
	if p, ok := r.Posta.Misure[1].Precisione(); !ok || p != 0.8 {
		t.Errorf("precision@1 di 20 agganci al primo su 25: %v, sufficiente %v", p, ok)
	}
	if p, ok := r.Posta.Misure[1].PrimoGiusto(); !ok || p < 0.666 || p > 0.667 {
		t.Errorf("primo giusto di 20 su 30 decisioni: %v, sufficiente %v", p, ok)
	}
	if _, ok := r.Posta.Misure[2].Precisione(); ok {
		t.Error("12 agganci non sono un campione, anche se le decisioni sono 22")
	}
	// senza dati: lo dice, non stampa tabelle vuote
	b.Reset()
	if err := (Rapporto{Host: "h:1", Database: "d"}).Scrivi(&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "nessuna decisione con la fotografia") || !strings.Contains(b.String(), "sola lettura: no") {
		t.Errorf("rapporto vuoto:\n%s", b.String())
	}
}
