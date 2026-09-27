package fascicolo

// L1 — B8.6: i codici candidati di una RFQ come regola pura (piano §6, §13 B8.6). Unisci non classifica:
// aggrega le evidenze gia' prodotte da candidato_codice, documento_proposta e componente_proposta per
// upper(codice), senza perderne nessuna e senza scegliere fra revisioni diverse; componenti,
// proposte STEP aperte e identificativi dicono soltanto se il codice e' gia' deciso. Le prove L4 con la
// vista vera stanno in candidati_db_test.go.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/db"
)

var mailDiProva = uuid.MustParse("11111111-1111-1111-1111-111111111111")

// dalMessaggio e' una riga di candidato_codice: famiglia o generico, e dove l'ha visto il triage.
func dalMessaggio(codice, rev, origine, dove string) db.ListCodiciCandidatiThreadRow {
	r := db.ListCodiciCandidatiThreadRow{Codice: codice, Rev: rev, Sorgente: SorgenteMessaggio, Origine: origine, Evidenza: dove,
		MessaggioID: mailDiProva, Ruolo: "non_classificato", Punteggio: 30}
	if origine == "famiglia" {
		r.Famiglia, r.Ruolo, r.Punteggio = "disegni 777", "prodotto", 80
	}
	return r
}

// dalDocumento e' il codice della proposta di un documento, con la sua fonte e il tipo del file.
func dalDocumento(codice, rev, fonte, file, tipo string, punti int32) db.ListCodiciCandidatiThreadRow {
	return db.ListCodiciCandidatiThreadRow{Codice: codice, Rev: rev, Sorgente: SorgenteDocumento, Origine: fonte, Punteggio: punti,
		Evidenza: file, TipoFile: tipo, MessaggioID: mailDiProva, AllegatoID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
}

// dalNome e' un codice trovato dentro il nome di un file che non e' esso stesso un codice.
func dalNome(codice, file, tipo string) db.ListCodiciCandidatiThreadRow {
	return db.ListCodiciCandidatiThreadRow{Codice: codice, Sorgente: SorgenteNomeFile, Origine: "nome_file", Punteggio: 50,
		Evidenza: file, TipoFile: tipo, MessaggioID: mailDiProva, AllegatoID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
}

// dalloStep e' un nodo di uno STEP, classificato dal server (componente_proposta).
func dalloStep(codice, rev, origine, file string) db.ListCodiciCandidatiThreadRow {
	r := db.ListCodiciCandidatiThreadRow{Codice: codice, Rev: rev, Sorgente: SorgenteStep, Origine: origine, Punteggio: 30,
		Evidenza: codice + " — " + file, MessaggioID: mailDiProva, AllegatoID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	if origine == "famiglia" {
		r.Famiglia, r.Punteggio = "disegni 777", 80
	}
	return r
}

func codiciDi(l []CodiceCandidato) string {
	s := make([]string, len(l))
	for i, c := range l {
		s[i] = c.Codice
	}
	return strings.Join(s, " ")
}

func trova(t *testing.T, c Candidati, codice string) CodiceCandidato {
	t.Helper()
	k, ok := c.Trova(codice)
	if !ok {
		t.Fatalf("%s non c'e' fra i candidati: prodotto [%s], altri [%s]", codice, codiciDi(c.Prodotto), codiciDi(c.Altri))
	}
	return k
}

// ---------------------------------------------------------------- le prove del piano

// Con famiglie dichiarate un numero visto solo dall'estrattore generico resta fra gli altri riferimenti,
// come in Proponibili; senza famiglie il generico e' tutto quello che c'e', ed e' un candidato.
func TestUnGenericoSoloNonEUnCandidatoProdottoSeCiSonoFamiglie(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{dalMessaggio("77722757", "", "famiglia", "oggetto"), dalMessaggio("20260908", "", "generico", "corpo")}
	con := Unisci(righe, ContestoCodici{HaFamiglie: true})
	if got := codiciDi(con.Prodotto); got != "77722757" {
		t.Errorf("candidati prodotto = [%s], atteso [77722757]", got)
	}
	if got := codiciDi(con.Altri); got != "20260908" {
		t.Fatalf("altri riferimenti = [%s], atteso [20260908]", got)
	}
	if m := con.Altri[0].Motivo; !strings.Contains(m, "solo dall'estrattore generico") || !strings.Contains(m, "il cliente ha famiglie") {
		t.Errorf("il motivo deve dire perche' non e' un candidato: %q", m)
	}
	senza := Unisci(righe, ContestoCodici{HaFamiglie: false})
	if got := codiciDi(senza.Prodotto); got != "77722757 20260908" || len(senza.Altri) != 0 {
		t.Errorf("senza famiglie: prodotto [%s], altri [%s]", got, codiciDi(senza.Altri))
	}
}

// Il nome di un file tecnico e' un'evidenza forte; lo stesso numero nel nome di un'offerta no (piano §6.1).
// Il cartiglio e la radice di uno STEP lo sono sempre.
//
// Riscritta per lo Smistamento (F4, `Unisci` con `nome_contiene_codice`): prima fissava gli altri riferimenti
// nell'ordine «12345678 1234567A», con il codice citato nel nome dell'offerta a 50 come la vista lo scrive.
// Adesso il suo score e' quello della regola della tabella S1 (25), e il codice del PDF da determinare (50)
// viene prima.
func TestIlNomeDiUnFileTecnicoEForteQuelloDiUnOffertaNo(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalDocumento("77720517", "", "nome_file", "77720517.dxf", "sviluppo_dxf", 90),
		dalNome("12345678", "Offerta 12345678 staffe.pdf", "commerciale"),
		dalDocumento("1234567A", "4", "estensione", "1234567A_4.pdf", "da_determinare", 50),
		dalDocumento("1234568A", "", "cartiglio", "foglio.pdf", "disegno_2d", 95),
	}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true})
	if got := codiciDi(c.Prodotto); got != "1234568A 77720517" {
		t.Errorf("candidati prodotto = [%s]", got)
	}
	if got := codiciDi(c.Altri); got != "1234567A 12345678" {
		t.Errorf("altri riferimenti = [%s]: un PDF ancora da determinare non e' un file tecnico", got)
	}
	if m := trova(t, c, "12345678").Motivo; m != "solo nel nome di un file non tecnico" {
		t.Errorf("motivo = %q", m)
	}
	// il codice citato nel nome vale lo score della sua regola, non il 50 fisso della vista
	if k := trova(t, c, "12345678"); k.Punteggio != classificazione.Punteggi["nome_contiene_codice"].Score || k.Punteggio != 25 ||
		k.Evidenze[0].Punteggio != 25 {
		t.Errorf("score del codice citato nel nome: %d (evidenza %d), atteso 25", k.Punteggio, k.Evidenze[0].Punteggio)
	}
	// le proposte dei documenti restano con la loro colonna, che e' gia' lo score del codice del file
	if k := trova(t, c, "1234567A"); k.Punteggio != 50 {
		t.Errorf("score del codice del documento: %d, atteso 50", k.Punteggio)
	}
}

// Lo stesso codice da un messaggio, dal nome di un disegno e da uno STEP e' UNA riga con tre evidenze:
// upper(codice) e' l'identita', e nessuna evidenza si perde.
func TestLoStessoCodiceDaTreFontiEUnaRigaConTreEvidenze(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalMessaggio("1234567A", "", "famiglia", "oggetto"),
		dalDocumento("1234567a", "4", "nome_file", "1234567a_4.pdf", "disegno_2d", 70),
		dalloStep("1234567A", "4", "famiglia", "assieme.stp"),
	}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true})
	if len(c.Prodotto) != 1 || len(c.Altri) != 0 {
		t.Fatalf("attesa una riga sola: prodotto [%s], altri [%s]", codiciDi(c.Prodotto), codiciDi(c.Altri))
	}
	k := c.Prodotto[0]
	if k.Chiave != "1234567A" || len(k.Evidenze) != 3 {
		t.Fatalf("chiave %q con %d evidenze, attese 3", k.Chiave, len(k.Evidenze))
	}
	sorgenti := map[string]bool{}
	for _, e := range k.Evidenze {
		sorgenti[e.Sorgente] = true
	}
	if !sorgenti[SorgenteMessaggio] || !sorgenti[SorgenteDocumento] || !sorgenti[SorgenteStep] {
		t.Errorf("sorgenti = %v", sorgenti)
	}
	if len(k.Revisioni) != 1 || k.Revisioni[0].Rev != "4" || k.Revisioni[0].Evidenze != 2 || k.Conflitto {
		t.Errorf("revisioni = %+v, conflitto %v: una sola, detta da due evidenze", k.Revisioni, k.Conflitto)
	}
	if k.Punteggio != 80 {
		t.Errorf("punteggio = %d, atteso il massimo (80)", k.Punteggio)
	}
}

// Il riferimento della richiesta non e' un codice: non c'e' quando arriva da candidato_codice (la vista lo
// esclude gia'), ne' quando lo stesso numero arriva dal nome di un file o da un'altra mail.
func TestIlRiferimentoNonCompareFraICodici(t *testing.T) {
	rif := dalMessaggio("RDO 400012345", "", "riferimento", "oggetto")
	rif.Ruolo = "riferimento_rfq"
	righe := []db.ListCodiciCandidatiThreadRow{
		rif,
		dalNome("400012345", "RDO 400012345.pdf", "da_determinare"),
		dalMessaggio("400012345", "", "generico", "corpo"),
		dalMessaggio("77722757", "", "famiglia", "oggetto"),
	}
	c := Unisci(righe, ContestoCodici{Riferimento: "RDO 400012345", HaFamiglie: true})
	if got := codiciDi(c.Prodotto) + "|" + codiciDi(c.Altri); got != "77722757|" {
		t.Errorf("codici = %q: il riferimento non deve comparire", got)
	}
}

// Un codice di famiglia visto solo nella storia citata era gia' deciso altrove: va fra gli altri. Lo
// stesso codice anche nel corpo nuovo e' un candidato, e tiene tutte e due le evidenze.
func TestUnCodiceDiFamigliaSoloNellaStoriaVaFraGliAltri(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalMessaggio("77722757", "", "famiglia", "storia citata"),
		dalMessaggio("77720517", "", "famiglia", "storia citata"),
		dalMessaggio("77720517", "", "famiglia", "corpo"),
	}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true})
	if got := codiciDi(c.Altri); got != "77722757" {
		t.Fatalf("altri riferimenti = [%s]", got)
	}
	if m := c.Altri[0].Motivo; m != "solo nella storia citata" {
		t.Errorf("motivo = %q", m)
	}
	k := trova(t, c, "77720517")
	if !k.Prodotto || len(k.Evidenze) != 2 {
		t.Errorf("77720517: prodotto %v con %d evidenze", k.Prodotto, len(k.Evidenze))
	}
}

// ---------------------------------------------------------------- revisioni

// Revisioni diverse sono un conflitto da mostrare: nessuna vince per punteggio. «b» e «B» sono la stessa;
// una revisione vuota non dice niente.
//
// Riscritta per lo Smistamento (R1, fase F2): prima fissava anche revisioneScelta, cioe' la revisione con
// cui «+ Prodotto/Assieme/Particolare» faceva nascere il componente dal codice trovato (senza scelta: rifiuto
// con l'elenco; «b» → B; «C» non vista → rifiuto; una sola → presa). Il gesto e AggiungiDaCodice sono tolti:
// la prova fissa adesso che il conflitto si legge intero (quante evidenze per revisione, il punteggio che
// ordina e non sceglie) e che niente, nella situazione del codice, porta una revisione scelta o un componente.
func TestLeRevisioniDiscordantiSonoUnConflittoNonUnPunteggio(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalDocumento("77722757", "A", "cartiglio", "77722757.pdf", "disegno_2d", 95),
		dalloStep("77722757", "B", "famiglia", "assieme.stp"),
		dalMessaggio("77722757", "b", "famiglia", "oggetto"),
		dalNome("77722757", "elenco 77722757.pdf", "disegno_2d"),
		dalMessaggio("77720517", "4", "famiglia", "corpo"),
		dalDocumento("77720517", "4", "nome_file", "77720517_4.pdf", "disegno_2d", 70),
	}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true})
	k := trova(t, c, "77722757")
	if !k.Conflitto || len(k.Revisioni) != 2 || k.Revisioni[0].Rev != "A" || k.Revisioni[1].Rev != "B" || k.Revisioni[1].Evidenze != 2 {
		t.Fatalf("revisioni = %+v, conflitto %v", k.Revisioni, k.Conflitto)
	}
	if k.Revisioni[0].Evidenze != 1 || k.Punteggio != 95 {
		t.Errorf("la A del cartiglio: %d evidenza, punteggio %d (il piu' alto: ordina la lista, non sceglie)", k.Revisioni[0].Evidenze, k.Punteggio)
	}
	if n := len(k.Evidenze); n != 4 {
		t.Errorf("77722757: %d evidenze, attese 4 (nessuna si perde per la revisione)", n)
	}
	if s := k.Stato; s.Situazione != SituazioneNuovo || s.Componente != nil {
		t.Errorf("un codice con revisioni discordanti resta un codice trovato, senza componente: %+v", s)
	}
	u := trova(t, c, "77720517")
	if u.Conflitto || len(u.Revisioni) != 1 {
		t.Errorf("77720517: %+v", u.Revisioni)
	}
	if u.Revisioni[0].Rev != "4" || u.Revisioni[0].Evidenze != 2 {
		t.Errorf("una revisione sola, detta da due evidenze: %+v", u.Revisioni)
	}
	if s := u.Stato; s.Situazione != SituazioneNuovo || s.Componente != nil {
		t.Errorf("77720517 resta un codice trovato: %+v", s)
	}
}

// ---------------------------------------------------------------- che cosa e' gia' deciso

func proposta(codice, file string) db.ListProposteNodoAperteRow {
	return db.ListProposteNodoAperteRow{PropostaID: uuid.New(), AllegatoID: uuid.New(), Chiave: "#2", Codice: codice, NomeGrezzo: codice,
		NomeFile: file, TipoProposto: db.NullTipoComponente{TipoComponente: db.TipoComponenteSottoassieme, Valid: true}}
}

func componente(codice string, tipo db.TipoComponente, archiviato bool) db.Componente {
	c := db.Componente{ComponenteID: uuid.New(), Codice: codice, Tipo: tipo}
	if archiviato {
		ieri := time.Now().Add(-24 * time.Hour)
		c.ArchiviatoIl = &ieri
	}
	return c
}

// Un codice con un nodo STEP aperto porta a quel nodo, anche se il componente c'e' ed e' archiviato.
//
// Riscritta per lo Smistamento (R1, fase F2): prima fissava anche che la riga non offrisse tipi per «+»
// (Stato.Tipi vuoto). Il campo non c'e' piu', perche' nessuna riga offre un gesto: la prova fissa che il
// nodo e' quello, e che il componente archiviato con lo stesso codice si vede accanto.
func TestUnCodiceConUnaPropostaStepApertaPortaAllaProposta(t *testing.T) {
	p := proposta("77720517", "assieme.stp")
	righe := []db.ListCodiciCandidatiThreadRow{dalloStep("77720517", "", "famiglia", "assieme.stp"), dalMessaggio("77720517", "", "famiglia", "corpo")}
	for _, comp := range [][]db.Componente{nil, {componente("77720517", db.TipoComponenteSciolto, true)}} {
		c := Unisci(righe, ContestoCodici{HaFamiglie: true, Proposte: []db.ListProposteNodoAperteRow{p}, Componenti: comp})
		s := trova(t, c, "77720517").Stato
		if s.Situazione != SituazioneProposta || s.Proposta().PropostaID != p.PropostaID || (s.Componente != nil) != (comp != nil) {
			t.Errorf("con %d componenti: situazione %s, proposta %v, componente %v", len(comp), s.Situazione, s.Proposta().PropostaID, s.Componente)
		}
	}
}

// Un codice che e' gia' un componente dice quale: le maiuscole non contano.
//
// Riscritta per lo Smistamento (R1, fase F2): prima fissava anche Stato.Tipi vuoto (nessun «+»); adesso
// nessuna riga ha tipi da offrire, e la prova fissa il componente trovato con la sua grafia.
func TestUnCodiceGiaComponenteSiApre(t *testing.T) {
	comp := componente("1234567a", db.TipoComponenteFinito, false)
	c := Unisci([]db.ListCodiciCandidatiThreadRow{dalMessaggio("1234567A", "", "famiglia", "oggetto")},
		ContestoCodici{HaFamiglie: true, Componenti: []db.Componente{comp}})
	s := trova(t, c, "1234567A").Stato
	if s.Situazione != SituazioneComponente || s.Componente == nil || s.Componente.ComponenteID != comp.ComponenteID || s.Componente.Codice != "1234567a" {
		t.Errorf("situazione %s, componente %v", s.Situazione, s.Componente)
	}
}

// Un codice di un componente archiviato dice che c'e', archiviato: stesso componente, stessa storia. Il
// ripristino sta negli Archiviati della Struttura BOM.
//
// Riscritta per lo Smistamento (R1, fase F2): prima «proponeva il ripristino» dal pannello dei codici
// (Stato.Tipi vuoto, bottone «Ripristina»); adesso la riga si legge e basta.
func TestUnCodiceArchiviatoProponeIlRipristino(t *testing.T) {
	comp := componente("77731111", db.TipoComponenteSciolto, true)
	c := Unisci([]db.ListCodiciCandidatiThreadRow{dalMessaggio("77731111", "", "famiglia", "corpo")},
		ContestoCodici{HaFamiglie: true, Componenti: []db.Componente{comp}})
	s := trova(t, c, "77731111").Stato
	if s.Situazione != SituazioneArchiviato || s.Componente.ComponenteID != comp.ComponenteID || s.Componente.ArchiviatoIl == nil {
		t.Errorf("situazione %s, componente %+v", s.Situazione, s.Componente)
	}
}

// Un codice nuovo e un codice della richiesta si leggono: il primo e' solo un codice trovato, il secondo
// e' gia' deciso come prodotto (nasce con AssicuraProdottiDellaRichiesta: la creazione della RFQ nel triage, o
// l'apertura di una revisione della BOM congelata).
//
// Riscritta per lo Smistamento (R1, fase F2): prima era TestSoloUnCodiceNuovoOffreITreTipi e fissava i tipi
// offerti ai «+» (prodotto, assieme, particolare per il nuovo; prodotto per quello della richiesta). Un
// codice trovato non fa piu' nascere un componente: la prova fissa le due situazioni, senza componente.
func TestUnCodiceNuovoEUnoDellaRichiestaSiLeggonoSoltanto(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{dalMessaggio("77760000", "", "famiglia", "corpo"), dalMessaggio("77750000", "", "famiglia", "oggetto")}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true, Identificativi: []db.IdentificativoThread{{Codice: "77750000"}}})
	nuovo := trova(t, c, "77760000").Stato
	if nuovo.Situazione != SituazioneNuovo || nuovo.Componente != nil || nuovo.Identificativo || len(nuovo.Proposte) != 0 {
		t.Errorf("nuovo: %+v", nuovo)
	}
	ric := trova(t, c, "77750000").Stato
	if ric.Situazione != SituazioneRichiesta || !ric.Identificativo || ric.Componente != nil || len(ric.Proposte) != 0 {
		t.Errorf("codice della richiesta: %+v", ric)
	}
}

// Con la BOM congelata la situazione resta quella e si mostra, e il pannello lo dice.
//
// Riscritta per lo Smistamento (R1, fase F2): prima fissava che a BOM congelata nessuna riga offrisse tipi
// per «+» (Stato.Tipi vuoto); adesso non ne offre nessuna mai. Fissa che ogni riga porta la versione
// congelata (il pannello la dice) e che la situazione non cambia.
func TestConLaBomCongelataNessunGestoCheLaCambia(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{dalMessaggio("77760000", "", "famiglia", "corpo"), dalMessaggio("77750000", "", "famiglia", "oggetto")}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true, Bloccata: 2, Identificativi: []db.IdentificativoThread{{Codice: "77750000"}}})
	if c.Bloccata != 2 {
		t.Errorf("Bloccata = %d", c.Bloccata)
	}
	for _, k := range append(c.Prodotto, c.Altri...) {
		if k.Stato.Componente != nil || k.Stato.Bloccata != 2 {
			t.Errorf("%s: componente %v, bloccata %d", k.Codice, k.Stato.Componente, k.Stato.Bloccata)
		}
	}
	if s := trova(t, c, "77760000").Stato.Situazione; s != SituazioneNuovo {
		t.Errorf("la situazione si mostra anche a BOM congelata: %s", s)
	}
}

// Stesse righe in un altro ordine, stesso risultato: Unisci e' pura.
func TestUnisciNonDipendeDallOrdineDelleRighe(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalMessaggio("77722757", "", "famiglia", "oggetto"), dalloStep("77722757", "B", "famiglia", "a.stp"),
		dalMessaggio("20260908", "", "generico", "corpo"), dalDocumento("77720517", "", "nome_file", "77720517.pdf", "disegno_2d", 70),
	}
	a := Unisci(righe, ContestoCodici{HaFamiglie: true})
	inverse := make([]db.ListCodiciCandidatiThreadRow, len(righe))
	for i := range righe {
		inverse[len(righe)-1-i] = righe[i]
	}
	b := Unisci(inverse, ContestoCodici{HaFamiglie: true})
	impronta := func(c Candidati) string {
		var s []string
		for _, k := range append(c.Prodotto, c.Altri...) {
			var ev []string
			for _, e := range k.Evidenze {
				ev = append(ev, e.Frase)
			}
			s = append(s, k.Codice+"="+strings.Join(ev, ";"))
		}
		return strings.Join(s, "|")
	}
	if impronta(a) != impronta(b) {
		t.Errorf("l'ordine delle righe cambia il risultato:\n%s\n%s", impronta(a), impronta(b))
	}
}

// Con i suffissi decorativi il codice «X» trovato nella RFQ e' il pezzo nato «X_PRT»: e' gia' nella BOM, e il
// pannello dei codici non offre di aggiungerne un secondo.
func TestUnCodiceTrovaIlPezzoConIlSuffisso(t *testing.T) {
	r, err := regole.ValidaRegole([]byte(`{"suffissi_decorativi": ["_PRT"]}`))
	if err != nil {
		t.Fatal(err)
	}
	vecchio := componente("77720000_PRT", db.TipoComponenteSciolto, false)
	righe := []db.ListCodiciCandidatiThreadRow{dalMessaggio("77720000", "", "generico", "corpo")}
	c := Unisci(righe, ContestoCodici{Componenti: []db.Componente{vecchio}, Motore: classificazione.Compila("ACME", r)})
	k := trova(t, c, "77720000")
	if k.Stato.Situazione != SituazioneComponente || k.Stato.Componente == nil || k.Stato.Componente.ComponenteID != vecchio.ComponenteID {
		t.Errorf("il codice 77720000: %+v", k.Stato)
	}
	c = Unisci(righe, ContestoCodici{Componenti: []db.Componente{vecchio}})
	if k := trova(t, c, "77720000"); k.Stato.Situazione != SituazioneNuovo {
		t.Errorf("senza la regola e' un codice nuovo: %+v", k.Stato)
	}
}
