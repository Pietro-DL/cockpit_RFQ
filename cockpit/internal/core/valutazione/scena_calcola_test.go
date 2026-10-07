// L1 — gli aiuti delle prove di Calcola (B6, V1 e V2; A1c-L1-14, A1c-L1-16, A1c-L1-20, A1c-L1-30): l'insieme delle
// regole ACME dalla porta del prodotto (CompilaInsieme, con l'indice e lo sha256 di ogni file), i thread di altri clienti
// inventati, la fotografia con più thread, il messaggio fuori RFQ di un caso di censimento, la permutazione degli
// elenchi della fotografia e dei casi; gli invarianti di ogni esito, con quelli dello smistamento (V2).
package valutazione_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I clienti di queste prove sono inventati, come in scena_test.go: ACME e tre suoi omonimi, con gli UUID
// 00000000-0000-4000-8000-00000000acNN; un cliente senza voce nell'indice, uno scartato, uno con la ragione sociale
// discorde.
var (
	clienteSenzaRegole = uuid.MustParse("00000000-0000-4000-8000-00000000ac03")
	clienteScartato    = uuid.MustParse("00000000-0000-4000-8000-00000000ac04")
	clienteDiscorde    = uuid.MustParse("00000000-0000-4000-8000-00000000ac05")

	threadSenzaRegole = uid(0x6a1)
	threadScartato    = uid(0x6a2)
	threadDiscorde    = uid(0x6a3)
	mSenzaRegole      = uid(0x6b1)
	mScartato         = uid(0x6b2)
	mDiscorde         = uid(0x6b3)
)

// grammaticaDi: i byte del file v1 di una grammatica, con il cliente, la ragione sociale e le famiglie date.
func grammaticaDi(t *testing.T, cliente uuid.UUID, ragione string, famiglie ...grammatica.FamigliaCodice) []byte {
	t.Helper()
	g := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: cliente, RagioneSociale: ragione},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       famiglie,
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// voceRegole: un cliente dell'indice delle regole, con il file della sua grammatica; sha "" = lo sha256 dei byte.
type voceRegole struct {
	cliente uuid.UUID
	file    string
	byte    []byte
	sha     string
}

// insiemeDi: l'insieme delle regole dalla porta del prodotto (motorea.CompilaInsieme), con i limiti ACME nell'indice.
func insiemeDi(t *testing.T, voci ...voceRegole) *motorea.InsiemeRegole {
	t.Helper()
	ind := grammatica.IndiceRegole{VersioneIndice: grammatica.VersioneIndice, Limiti: limitiACME()}
	contenuti := map[string][]byte{}
	for _, v := range voci {
		s := v.sha
		if s == "" {
			h := sha256.Sum256(v.byte)
			s = hex.EncodeToString(h[:])
		}
		ind.Grammatiche = append(ind.Grammatiche, grammatica.VoceIndice{ClienteID: v.cliente, File: v.file, Sha256: s})
		contenuti[v.file] = v.byte
	}
	r, _, err := motorea.CompilaInsieme(ind, contenuti)
	if err != nil {
		t.Fatalf("CompilaInsieme: %v", err)
	}
	return &r
}

// insiemeACME: ACME con la famiglia della catena; il cliente scartato (lo sha256 dell'indice non è quello del file:
// regole.sha256_discorde); il cliente discorde, con una grammatica valida che dice un'altra ragione sociale. Il cliente
// senza regole non ha una voce.
func insiemeACME(t *testing.T) *motorea.InsiemeRegole {
	t.Helper()
	r := insiemeDi(t,
		voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", famigliaCatena())},
		voceRegole{cliente: clienteScartato, file: "scartato.v1.json", byte: grammaticaDi(t, clienteScartato, "ACME Scartata S.r.l.", famigliaCatena()),
			sha: strings.Repeat("0", 64)},
		voceRegole{cliente: clienteDiscorde, file: "discorde.v1.json", byte: grammaticaDi(t, clienteDiscorde, "ACME Uno S.r.l.", famigliaCatena())},
	)
	if r.Motori[clienteACME] == nil || r.Motori[clienteDiscorde] == nil || r.Scartati[clienteScartato] == nil {
		t.Fatalf("insieme ACME: motori %d, scartati %d", len(r.Motori), len(r.Scartati))
	}
	return r
}

// threadDi: una RFQ di un altro cliente, con un messaggio con il gesto 1 e il corpo dato.
func threadDi(id, cliente, msg uuid.UUID, corpo string) fotorfq.Thread {
	m := messaggio(msg, 0, "Richiesta di offerta", corpo)
	th, c := id, cliente
	m.ThreadID, m.ControparteClienteID = &th, &c
	return fotorfq.Thread{ID: id, ClienteID: cliente, Stato: "APERTA", CreatoIl: dataACME, Messaggi: []fotorfq.Messaggio{m},
		Agganci: []fotorfq.AggancioMessaggio{gesto(msg)}}
}

// fotografiaDi: la fotografia con i thread dati, letta come dal caricatore, con i quattro clienti: il cliente discorde
// ha nel DB una ragione sociale diversa da quella della sua grammatica.
func fotografiaDi(thread ...fotorfq.Thread) fotorfq.Fotografia {
	f := fotografia(thread[0])
	f.Thread = thread
	f.Clienti = []fotorfq.Cliente{{ID: clienteACME, RagioneSociale: "ACME S.p.A."}, {ID: clienteSenzaRegole, RagioneSociale: "ACME Tre S.r.l."},
		{ID: clienteScartato, RagioneSociale: "ACME Scartata S.r.l."}, {ID: clienteDiscorde, RagioneSociale: "ACME Due S.r.l."}}
	return f
}

// quattroThread: la RFQ ACME della scena del collegamento e le tre RFQ degli altri clienti, che chiedono lo stesso
// codice nel corpo.
func quattroThread(t *testing.T) []fotorfq.Thread {
	t.Helper()
	corpo := "Buongiorno,\r\nvi chiediamo l'offerta per 7120100A2.\r\nGrazie"
	return []fotorfq.Thread{scenaCollegamento(t), threadDi(threadSenzaRegole, clienteSenzaRegole, mSenzaRegole, corpo),
		threadDi(threadScartato, clienteScartato, mScartato, corpo), threadDi(threadDiscorde, clienteDiscorde, mDiscorde, corpo)}
}

// calcola chiama Calcola e controlla gli invarianti di ogni esito: le versioni, i thread in ordine di ID, un record dei
// file e un record piatto per allegato in ordine di AllegatoID, lo smistamento di V2 (invariantiSmistamento), i prodotti e
// il fascicolo di V3 (invariantiDelProdotto, PO-35 su tutti i casi; coerenzaDelleFonti, T-B2-02; invariantiDelFascicolo),
// le diagnostiche in ordine, l'impronta come canonico dell'esito con l'impronta vuota.
func calcola(t *testing.T, f fotorfq.Fotografia, r *motorea.InsiemeRegole, in valutazione.Ingressi) valutazione.Esito {
	t.Helper()
	e, err := valutazione.Calcola(f, r, in)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if e.VersioneValutazione != valutazione.VersioneValutazione || e.VersioneImprontaProdotto != valutazione.VersioneImprontaProdotto ||
		e.VersioneFormati2D != valutazione.VersioneFormati2D || e.VersioneComposizione != motorea.VersioneComposizione {
		t.Errorf("versioni %q %d %d %q", e.VersioneValutazione, e.VersioneImprontaProdotto, e.VersioneFormati2D, e.VersioneComposizione)
	}
	for i, et := range e.Thread {
		if i > 0 && e.Thread[i-1].ThreadID.String() >= et.ThreadID.String() {
			t.Errorf("thread fuori ordine: %s prima di %s", e.Thread[i-1].ThreadID, et.ThreadID)
		}
		th := threadDiID(t, f, et.ThreadID)
		if len(et.File) != len(th.Allegati) || len(et.Confrontabili) != len(th.Allegati) {
			t.Errorf("thread %s: %d record dei file e %d record piatti per %d allegati", et.ThreadID, len(et.File), len(et.Confrontabili), len(th.Allegati))
		}
		for j := 1; j < len(et.File); j++ {
			if et.File[j-1].AllegatoID.String() >= et.File[j].AllegatoID.String() ||
				et.Confrontabili[j-1].AllegatoID.String() >= et.Confrontabili[j].AllegatoID.String() {
				t.Errorf("thread %s: record fuori ordine di allegato", et.ThreadID)
			}
		}
		for _, pv := range et.ProdottiValutati {
			invariantiDelProdotto(t, f, et.Valutato, pv)
		}
		coerenzaDelleFonti(t, et.ProdottiValutati, et.Ancoraggi.Strutture)
		invariantiDelFascicolo(t, f, et, th)
		invariantiSmistamento(t, f, et, th)
		for j := 1; j < len(et.Diagnostiche); j++ {
			if et.Diagnostiche[j-1].Codice > et.Diagnostiche[j].Codice {
				t.Errorf("thread %s: diagnostiche fuori ordine", et.ThreadID)
			}
		}
	}
	senza := e
	senza.Impronta = ""
	if h, err := jsoncanonico.ImprontaDi(senza); err != nil || h != e.Impronta || len(e.Impronta) != 64 {
		t.Errorf("impronta %q, il canonico dell'esito dà %q (%v)", e.Impronta, h, err)
	}
	return e
}

// invariantiSmistamento: gli invarianti dello smistamento di un thread (B6, V2; contratto §1.5; F.3, F0-12, F0-17,
// F0-18; T-B6-07, T-B6-09, T-E1R-10):
//   - un thread con un errore della valutazione (nessun prodotto, motivo errore_valutazione) non ha associazioni né file
//     da smistare; altrimenti un'associazione per allegato, in ordine di AllegatoID;
//   - un'esclusione è terminale; PertinenzaContesto è dentro Pertinente; un file fuori perimetro non è pertinente;
//   - il contesto del messaggio (R106 B e R107, precisate il 07/10) solo per un file che conta, con il messaggio, le
//     letture e i prodotti, in ordine e senza doppioni; PertinenzaContesto e ProdottiContesto vengono da lui;
//     valutazione.contesto_discorde solo per un file con il contesto, avviso, con i prodotti nominati del contesto e
//     nessuno di quelli collegati fra loro (quelli delle evidenze dentro Pertinente);
//   - «da smistare» in ordine di allegato, con uno dei cinque motivi, solo file che contano e non sono terminali;
//     l'orfano senza prodotti, i prodotti del contesto solo per un orfano, e mai uno solo; senza prodotti è orfano se la
//     pertinenza si calcola, e nessun file è orfano se non si calcola (R-75 della revisione di V2: senza grammatica, o
//     con una sezione dello smistamento assente);
//   - gli orfani e gli avvisi del fascicolo vengono da «da smistare»;
//   - lo smistamento di ogni prodotto: mai verificato senza calcolo (T-12, il target senza componente, il thread senza
//     grammatica), né in un thread non valutato; verificato solo senza file non terminali, motivi, conflitti; nessun
//     percorso (Calcola non ne passa: LD-18); i file non terminali in ordine; i conflitti dell'asse con un prodotto del
//     thread (o vuoto).
func invariantiSmistamento(t *testing.T, f fotorfq.Fotografia, et valutazione.EsitoThread, th fotorfq.Thread) {
	t.Helper()
	pertinenzaCalcolata := et.HashSnapshot != ""
	for _, k := range valutazione.SezioniDelloSmistamentoPerProva {
		if s, ok := f.Sezioni[k]; f.Sezioni != nil && (!ok || s.Stato == fotorfq.StatoSezioneAssente) {
			pertinenzaCalcolata = false
		}
	}
	errore := et.Motivo == valutazione.MotivoThreadErroreValutazione && len(et.ProdottiValutati) == 0
	switch {
	case errore && (len(et.Associazioni) != 0 || len(et.DaSmistare) != 0):
		t.Errorf("thread %s con un errore: %d associazioni, %d file da smistare", et.ThreadID, len(et.Associazioni), len(et.DaSmistare))
	case !errore && len(et.Associazioni) != len(th.Allegati):
		t.Errorf("thread %s: %d associazioni per %d allegati", et.ThreadID, len(et.Associazioni), len(th.Allegati))
	}
	per := map[uuid.UUID]valutazione.AssociazioneFile{}
	for i, a := range et.Associazioni {
		per[a.AllegatoID] = a
		if i > 0 && et.Associazioni[i-1].AllegatoID.String() >= a.AllegatoID.String() {
			t.Errorf("thread %s: associazioni fuori ordine", et.ThreadID)
		}
		if a.Esclusa && !a.Terminale {
			t.Errorf("thread %s, file %s: escluso ma non terminale (T-B0-32)", et.ThreadID, a.AllegatoID)
		}
		for _, p := range a.PertinenzaContesto {
			if !contieneRif(a.Pertinente, p) {
				t.Errorf("thread %s, file %s: il contesto %s non è fra i pertinenti %v", et.ThreadID, a.AllegatoID, p, a.Pertinente)
			}
		}
		if a.Perimetro != valutazione.PerimetroDentro && len(a.Pertinente) != 0 {
			t.Errorf("thread %s, file %s: fuori perimetro (%s) ma pertinente a %v", et.ThreadID, a.AllegatoID, a.Perimetro, a.Pertinente)
		}
		if c := a.Contesto; c != nil {
			if a.Perimetro != valutazione.PerimetroDentro || c.MessaggioID == uuid.Nil || len(c.Letture) == 0 || len(c.Prodotti) == 0 ||
				!ordinatoSenzaDoppioni(c.Letture) || !ordinatoSenzaDoppioni(c.Prodotti) {
				t.Errorf("thread %s, file %s: contesto %+v (perimetro %s)", et.ThreadID, a.AllegatoID, c, a.Perimetro)
			}
		}
		for _, p := range a.PertinenzaContesto {
			if a.Contesto == nil || !contieneRif(a.Contesto.Prodotti, p) {
				t.Errorf("thread %s, file %s: la pertinenza per il contesto %s senza il contesto %+v", et.ThreadID, a.AllegatoID, p, a.Contesto)
			}
		}
	}
	for _, d := range et.Diagnostiche {
		if d.Codice != valutazione.CodiceContestoDiscorde {
			continue
		}
		if d.Gravita != evidenze.GravitaAvviso || len(d.Rif) == 0 {
			t.Errorf("thread %s: contesto_discorde %+v", et.ThreadID, d)
			continue
		}
		a, ok := per[uuid.MustParse(d.Rif[0])]
		var nominati, collegati []string
		for _, r := range d.Rif[1:] {
			switch {
			case strings.HasPrefix(r, "contesto:"):
				nominati = append(nominati, strings.TrimPrefix(r, "contesto:"))
			case strings.HasPrefix(r, "evidenza:"):
				collegati = append(collegati, strings.TrimPrefix(r, "evidenza:"))
				if !contieneRif(a.Pertinente, strings.TrimPrefix(r, "evidenza:")) {
					t.Errorf("thread %s: contesto_discorde con un'evidenza fuori dai pertinenti: %+v, %+v", et.ThreadID, d, a)
				}
			case strings.HasPrefix(r, "decisione:") && a.Terminale:
				collegati = append(collegati, strings.TrimPrefix(r, "decisione:"))
			default:
				t.Errorf("thread %s: contesto_discorde con il riferimento %q", et.ThreadID, r)
			}
		}
		if !ok || a.Contesto == nil || !reflect.DeepEqual(nominati, a.Contesto.Prodotti) || len(collegati) == 0 {
			t.Errorf("thread %s: contesto_discorde %+v senza il contesto dell'associazione %+v", et.ThreadID, d, a)
			continue
		}
		for _, p := range collegati {
			if contieneRif(nominati, p) {
				t.Errorf("thread %s: contesto_discorde con un prodotto in comune: %+v", et.ThreadID, d)
			}
		}
	}
	cinque := map[valutazione.MotivoSmistamento]bool{valutazione.MotivoSmistamentoNessunCandidato: true, valutazione.MotivoSmistamentoAssociazioneAmbigua: true,
		valutazione.MotivoSmistamentoAssociazioneDiscordante: true, valutazione.MotivoSmistamentoCollocazioneNonDeterminabile: true,
		valutazione.MotivoSmistamentoFuoriRichiestaNonConfermato: true}
	orfani := 0
	for i, d := range et.DaSmistare {
		if i > 0 && et.DaSmistare[i-1].AllegatoID.String() >= d.AllegatoID.String() {
			t.Errorf("thread %s: file da smistare fuori ordine", et.ThreadID)
		}
		a := per[d.AllegatoID]
		if !cinque[d.Motivo] || a.Terminale || a.Perimetro != valutazione.PerimetroDentro || (d.Orfano && len(d.Prodotti) != 0) ||
			(len(d.Prodotti) == 0 && d.Orfano != pertinenzaCalcolata) ||
			(!d.Orfano && len(d.ProdottiContesto) > 0) || len(d.ProdottiContesto) == 1 {
			t.Errorf("thread %s: file da smistare %+v con l'associazione %+v", et.ThreadID, d, a)
		}
		if d.Orfano {
			orfani++
		}
		if len(d.ProdottiContesto) > 0 && (a.Contesto == nil || !reflect.DeepEqual(d.ProdottiContesto, a.Contesto.Prodotti)) {
			t.Errorf("thread %s: i prodotti del contesto dell'orfano %v senza il contesto %+v", et.ThreadID, d.ProdottiContesto, a.Contesto)
		}
	}
	avvisiOrfani := 0
	for _, a := range et.Fascicolo.Avvisi {
		if strings.HasPrefix(a, "orfano:") {
			avvisiOrfani++
		}
	}
	if et.Fascicolo.Orfani != orfani || avvisiOrfani != orfani {
		t.Errorf("thread %s: %d orfani e %d avvisi di orfani nel fascicolo, %d orfani da smistare", et.ThreadID, et.Fascicolo.Orfani, avvisiOrfani, orfani)
	}
	rif := map[string]bool{"": true}
	for _, pv := range et.ProdottiValutati {
		rif[pv.Rif] = true
		s := pv.Smistamento
		switch {
		case !s.Calcolata && s.Stato != valutazione.SmistamentoDaVerificare:
			t.Errorf("thread %s, prodotto %s: smistamento %s senza calcolo", et.ThreadID, pv.Rif, s.Stato)
		case !et.Valutato && s.Stato == valutazione.SmistamentoVerificato:
			t.Errorf("thread %s non valutato, prodotto %s: smistamento verificato (T-B6-09)", et.ThreadID, pv.Rif)
		case s.Stato == valutazione.SmistamentoVerificato && (len(s.FileNonTerminali)+len(s.Motivi)+len(s.Conflitti)+len(s.Percorsi) != 0):
			t.Errorf("thread %s, prodotto %s: verificato con %+v", et.ThreadID, pv.Rif, s)
		case len(s.Percorsi) != 0 || s.Stato == valutazione.SmistamentoInRevisione:
			t.Errorf("thread %s, prodotto %s: percorsi %v senza un adattatore (LD-18)", et.ThreadID, pv.Rif, s.Percorsi)
		}
		for j := 1; j < len(s.FileNonTerminali); j++ {
			if s.FileNonTerminali[j-1].String() >= s.FileNonTerminali[j].String() {
				t.Errorf("thread %s, prodotto %s: file non terminali fuori ordine", et.ThreadID, pv.Rif)
			}
		}
	}
	for _, c := range et.Conflitti {
		if c.Asse == valutazione.AsseSmistamento && !rif[c.Prodotto] {
			t.Errorf("thread %s: conflitto dello smistamento su un prodotto che non c'è: %+v", et.ThreadID, c)
		}
	}
}

// ordinatoSenzaDoppioni: l'elenco è in ordine di byte, senza doppioni.
func ordinatoSenzaDoppioni(elenco []string) bool {
	for i := 1; i < len(elenco); i++ {
		if elenco[i-1] >= elenco[i] {
			return false
		}
	}
	return true
}

// contieneRif: l'elenco ha quel Rif.
func contieneRif(elenco []string, s string) bool {
	for _, x := range elenco {
		if x == s {
			return true
		}
	}
	return false
}

func threadDiID(t *testing.T, f fotorfq.Fotografia, id uuid.UUID) fotorfq.Thread {
	t.Helper()
	for _, th := range f.Thread {
		if th.ID == id {
			return th
		}
	}
	t.Fatalf("il thread %s non è nella fotografia", id)
	return fotorfq.Thread{}
}

func esitoThread(t *testing.T, e valutazione.Esito, id uuid.UUID) valutazione.EsitoThread {
	t.Helper()
	for _, et := range e.Thread {
		if et.ThreadID == id {
			return et
		}
	}
	t.Fatalf("il thread %s non è nell'esito", id)
	return valutazione.EsitoThread{}
}

func confrontabileDi(t *testing.T, et valutazione.EsitoThread, allegato uuid.UUID) valutazione.FileConfrontabile {
	t.Helper()
	for _, c := range et.Confrontabili {
		if c.AllegatoID == allegato {
			return c
		}
	}
	t.Fatalf("nessun record piatto per l'allegato %s", allegato)
	return valutazione.FileConfrontabile{}
}

func fileInterpretatoDi(t *testing.T, et valutazione.EsitoThread, allegato uuid.UUID) valutazione.FileInterpretato {
	t.Helper()
	for _, f := range et.File {
		if f.AllegatoID == allegato {
			return f
		}
	}
	t.Fatalf("nessun record del file per l'allegato %s", allegato)
	return valutazione.FileInterpretato{}
}

// codiciDi: i codici delle diagnostiche, in ordine.
func codiciDi(d []evidenze.Diagnostica) []string {
	var out []string
	for _, x := range d {
		out = append(out, x.Codice)
	}
	return out
}

// messaggioFuori: un messaggio senza RFQ del cliente dato, agganciato in automatico, con gli allegati dati e i loro fatti.
func messaggioFuori(id, cliente uuid.UUID, corpo string, allegati []fotorfq.Allegato, fatti ...fotorfq.Fatti) fotorfq.MessaggioFuoriRFQ {
	m := messaggio(id, 30, "Ordine ACME", corpo)
	c := cliente
	m.ThreadID, m.ControparteClienteID = nil, &c
	mf := fotorfq.MessaggioFuoriRFQ{Messaggio: m, Aggancio: fotorfq.AggancioMessaggio{MessaggioID: id, Aggancio: "automatico"}}
	for _, a := range allegati {
		a.MessaggioID = id
		mf.Allegati = append(mf.Allegati, a)
	}
	for _, f := range fatti {
		if mf.Fatti == nil {
			mf.Fatti = map[string]fotorfq.Fatti{}
		}
		mf.Fatti[f.Sha256] = f
	}
	return mf
}

// permuta: la stessa fotografia e gli stessi casi con ogni elenco rovesciato, compresi gli elenchi dentro i thread.
func permuta(f fotorfq.Fotografia, in valutazione.Ingressi) (fotorfq.Fotografia, valutazione.Ingressi) {
	g := f
	g.Thread = nil
	for _, th := range rovescia(f.Thread) {
		th.Messaggi, th.Agganci, th.Allegati = rovescia(th.Messaggi), rovescia(th.Agganci), rovescia(th.Allegati)
		th.Proposte, th.Documenti, th.Identificativi = rovescia(th.Proposte), rovescia(th.Documenti), rovescia(th.Identificativi)
		th.Componenti, th.Relazioni = rovescia(th.Componenti), rovescia(th.Relazioni)
		th.RigheComponenteProposta, th.RigheRelazioneProposta = rovescia(th.RigheComponenteProposta), rovescia(th.RigheRelazioneProposta)
		th.StepProdotto, th.CandidatiCodice, th.InAttesa = rovescia(th.StepProdotto), rovescia(th.CandidatiCodice), rovescia(th.InAttesa)
		g.Thread = append(g.Thread, th)
	}
	g.Clienti, g.FuoriRFQ = rovescia(f.Clienti), nil
	for _, mf := range rovescia(f.FuoriRFQ) {
		mf.Allegati = rovescia(mf.Allegati)
		g.FuoriRFQ = append(g.FuoriRFQ, mf)
	}
	out := in
	out.Casi = nil
	for _, c := range rovescia(in.Casi) {
		c.Messaggi = rovescia(c.Messaggi)
		out.Casi = append(out.Casi, c)
	}
	return g, out
}
