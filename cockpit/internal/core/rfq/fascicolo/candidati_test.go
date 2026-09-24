package fascicolo

// L1 — B8.6: i codici candidati di una RFQ come regola pura (piano §6, §13 B8.6). Unisci non classifica:
// aggrega le evidenze gia' prodotte da candidato_codice, documento_proposta e componente_proposta per
// upper(codice), senza perderne nessuna e senza scegliere fra revisioni diverse; componenti,
// proposte STEP aperte e identificativi dicono soltanto se il codice e' gia' deciso. Le prove L4 con la
// vista vera stanno in candidati_db_test.go.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

var mailDiProva = uuid.MustParse("11111111-1111-1111-1111-111111111111")

// dalMessaggio e' una riga di candidato_codice: famiglia o generico, e dove l'ha visto il triage.
func dalMessaggio(codice, rev, origine, dove string) db.ListCodiciCandidatiThreadRow {
	r := db.ListCodiciCandidatiThreadRow{Codice: codice, Rev: rev, Sorgente: SorgenteMessaggio, Origine: origine, Evidenza: dove,
		MessaggioID: mailDiProva, Ruolo: "non_classificato", Punteggio: 30}
	if origine == "famiglia" {
		r.Famiglia, r.Ruolo, r.Punteggio = "disegni 529", "prodotto", 80
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
		r.Famiglia, r.Punteggio = "disegni 529", 80
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
	righe := []db.ListCodiciCandidatiThreadRow{dalMessaggio("52922757", "", "famiglia", "oggetto"), dalMessaggio("20260908", "", "generico", "corpo")}
	con := Unisci(righe, ContestoCodici{HaFamiglie: true})
	if got := codiciDi(con.Prodotto); got != "52922757" {
		t.Errorf("candidati prodotto = [%s], atteso [52922757]", got)
	}
	if got := codiciDi(con.Altri); got != "20260908" {
		t.Fatalf("altri riferimenti = [%s], atteso [20260908]", got)
	}
	if m := con.Altri[0].Motivo; !strings.Contains(m, "solo dall'estrattore generico") || !strings.Contains(m, "il cliente ha famiglie") {
		t.Errorf("il motivo deve dire perche' non e' un candidato: %q", m)
	}
	senza := Unisci(righe, ContestoCodici{HaFamiglie: false})
	if got := codiciDi(senza.Prodotto); got != "52922757 20260908" || len(senza.Altri) != 0 {
		t.Errorf("senza famiglie: prodotto [%s], altri [%s]", got, codiciDi(senza.Altri))
	}
}

// Il nome di un file tecnico e' un'evidenza forte; lo stesso numero nel nome di un'offerta no (piano §6.1).
// Il cartiglio e la radice di uno STEP lo sono sempre.
func TestIlNomeDiUnFileTecnicoEForteQuelloDiUnOffertaNo(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalDocumento("52920517", "", "nome_file", "52920517.dxf", "sviluppo_dxf", 90),
		dalNome("12345678", "Offerta 12345678 staffe.pdf", "commerciale"),
		dalDocumento("6674611A", "4", "estensione", "6674611A_4.pdf", "da_determinare", 50),
		dalDocumento("6674612A", "", "cartiglio", "foglio.pdf", "disegno_2d", 95),
	}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true})
	if got := codiciDi(c.Prodotto); got != "6674612A 52920517" {
		t.Errorf("candidati prodotto = [%s]", got)
	}
	if got := codiciDi(c.Altri); got != "12345678 6674611A" {
		t.Errorf("altri riferimenti = [%s]: un PDF ancora da determinare non e' un file tecnico", got)
	}
	if m := trova(t, c, "12345678").Motivo; m != "solo nel nome di un file non tecnico" {
		t.Errorf("motivo = %q", m)
	}
}

// Lo stesso codice da un messaggio, dal nome di un disegno e da uno STEP e' UNA riga con tre evidenze:
// upper(codice) e' l'identita', e nessuna evidenza si perde.
func TestLoStessoCodiceDaTreFontiEUnaRigaConTreEvidenze(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalMessaggio("6674611A", "", "famiglia", "oggetto"),
		dalDocumento("6674611a", "4", "nome_file", "6674611a_4.pdf", "disegno_2d", 70),
		dalloStep("6674611A", "4", "famiglia", "assieme.stp"),
	}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true})
	if len(c.Prodotto) != 1 || len(c.Altri) != 0 {
		t.Fatalf("attesa una riga sola: prodotto [%s], altri [%s]", codiciDi(c.Prodotto), codiciDi(c.Altri))
	}
	k := c.Prodotto[0]
	if k.Chiave != "6674611A" || len(k.Evidenze) != 3 {
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
	rif := dalMessaggio("RDO 490020618", "", "riferimento", "oggetto")
	rif.Ruolo = "riferimento_rfq"
	righe := []db.ListCodiciCandidatiThreadRow{
		rif,
		dalNome("490020618", "RDO 490020618.pdf", "da_determinare"),
		dalMessaggio("490020618", "", "generico", "corpo"),
		dalMessaggio("52922757", "", "famiglia", "oggetto"),
	}
	c := Unisci(righe, ContestoCodici{Riferimento: "RDO 490020618", HaFamiglie: true})
	if got := codiciDi(c.Prodotto) + "|" + codiciDi(c.Altri); got != "52922757|" {
		t.Errorf("codici = %q: il riferimento non deve comparire", got)
	}
}

// Un codice di famiglia visto solo nella storia citata era gia' deciso altrove: va fra gli altri. Lo
// stesso codice anche nel corpo nuovo e' un candidato, e tiene tutte e due le evidenze.
func TestUnCodiceDiFamigliaSoloNellaStoriaVaFraGliAltri(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalMessaggio("52922757", "", "famiglia", "storia citata"),
		dalMessaggio("52920517", "", "famiglia", "storia citata"),
		dalMessaggio("52920517", "", "famiglia", "corpo"),
	}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true})
	if got := codiciDi(c.Altri); got != "52922757" {
		t.Fatalf("altri riferimenti = [%s]", got)
	}
	if m := c.Altri[0].Motivo; m != "solo nella storia citata" {
		t.Errorf("motivo = %q", m)
	}
	k := trova(t, c, "52920517")
	if !k.Prodotto || len(k.Evidenze) != 2 {
		t.Errorf("52920517: prodotto %v con %d evidenze", k.Prodotto, len(k.Evidenze))
	}
}

// ---------------------------------------------------------------- revisioni

// Revisioni diverse sono un conflitto da mostrare: nessuna vince per punteggio, e chi aggiunge sceglie
// fra quelle viste. «b» e «B» sono la stessa; una revisione vuota non dice niente.
func TestLeRevisioniDiscordantiSonoUnConflittoNonUnPunteggio(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalDocumento("52922757", "A", "cartiglio", "52922757.pdf", "disegno_2d", 95),
		dalloStep("52922757", "B", "famiglia", "assieme.stp"),
		dalMessaggio("52922757", "b", "famiglia", "oggetto"),
		dalNome("52922757", "elenco 52922757.pdf", "disegno_2d"),
		dalMessaggio("52920517", "4", "famiglia", "corpo"),
		dalDocumento("52920517", "4", "nome_file", "52920517_4.pdf", "disegno_2d", 70),
	}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true})
	k := trova(t, c, "52922757")
	if !k.Conflitto || len(k.Revisioni) != 2 || k.Revisioni[0].Rev != "A" || k.Revisioni[1].Rev != "B" || k.Revisioni[1].Evidenze != 2 {
		t.Fatalf("revisioni = %+v, conflitto %v", k.Revisioni, k.Conflitto)
	}
	var r Rifiuto
	if _, err := revisioneScelta(k, ""); !errors.As(err, &r) || !strings.Contains(string(r), "revisioni discordanti") ||
		!strings.Contains(string(r), "A (1 evidenza)") || !strings.Contains(string(r), "B (2 evidenze)") {
		t.Errorf("senza scelta: %v (la A del cartiglio ha il punteggio piu' alto, e non deve vincere)", err)
	}
	if rev, err := revisioneScelta(k, "b"); err != nil || rev != "B" {
		t.Errorf("scelta «b»: %q, %v", rev, err)
	}
	if _, err := revisioneScelta(k, "C"); !errors.As(err, &r) || !strings.Contains(string(r), "non è fra quelle viste") {
		t.Errorf("scelta «C»: %v", err)
	}
	u := trova(t, c, "52920517")
	if u.Conflitto || len(u.Revisioni) != 1 {
		t.Errorf("52920517: %+v", u.Revisioni)
	}
	if rev, err := revisioneScelta(u, ""); err != nil || rev != "4" {
		t.Errorf("una revisione sola si prende senza chiedere: %q, %v", rev, err)
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

// Un codice con un nodo STEP aperto porta a quel nodo, anche se il componente c'e' ed e' archiviato:
// accettarlo lo ripristina, e porta con se' il file. Niente «+» da qui: sarebbe un componente parallelo.
func TestUnCodiceConUnaPropostaStepApertaPortaAllaProposta(t *testing.T) {
	p := proposta("52920517", "assieme.stp")
	righe := []db.ListCodiciCandidatiThreadRow{dalloStep("52920517", "", "famiglia", "assieme.stp"), dalMessaggio("52920517", "", "famiglia", "corpo")}
	for _, comp := range [][]db.Componente{nil, {componente("52920517", db.TipoComponenteSciolto, true)}} {
		c := Unisci(righe, ContestoCodici{HaFamiglie: true, Proposte: []db.ListProposteNodoAperteRow{p}, Componenti: comp})
		s := trova(t, c, "52920517").Stato
		if s.Situazione != SituazioneProposta || s.Proposta().PropostaID != p.PropostaID || len(s.Tipi) != 0 {
			t.Errorf("con %d componenti: situazione %s, proposta %v, tipi %v", len(comp), s.Situazione, s.Proposta().PropostaID, s.Tipi)
		}
	}
}

// Un codice che e' gia' un componente si apre: le maiuscole non contano.
func TestUnCodiceGiaComponenteSiApre(t *testing.T) {
	comp := componente("6674611a", db.TipoComponenteFinito, false)
	c := Unisci([]db.ListCodiciCandidatiThreadRow{dalMessaggio("6674611A", "", "famiglia", "oggetto")},
		ContestoCodici{HaFamiglie: true, Componenti: []db.Componente{comp}})
	s := trova(t, c, "6674611A").Stato
	if s.Situazione != SituazioneComponente || s.Componente == nil || s.Componente.ComponenteID != comp.ComponenteID || len(s.Tipi) != 0 {
		t.Errorf("situazione %s, componente %v, tipi %v", s.Situazione, s.Componente, s.Tipi)
	}
}

// Un codice di un componente archiviato propone il ripristino: stesso componente, stessa storia.
func TestUnCodiceArchiviatoProponeIlRipristino(t *testing.T) {
	comp := componente("52931111", db.TipoComponenteSciolto, true)
	c := Unisci([]db.ListCodiciCandidatiThreadRow{dalMessaggio("52931111", "", "famiglia", "corpo")},
		ContestoCodici{HaFamiglie: true, Componenti: []db.Componente{comp}})
	s := trova(t, c, "52931111").Stato
	if s.Situazione != SituazioneArchiviato || s.Componente.ComponenteID != comp.ComponenteID || len(s.Tipi) != 0 {
		t.Errorf("situazione %s, tipi %v", s.Situazione, s.Tipi)
	}
}

// Solo un codice davvero nuovo offre prodotto, assieme e particolare. Un codice della richiesta e' gia'
// deciso come prodotto: entra come prodotto.
func TestSoloUnCodiceNuovoOffreITreTipi(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{dalMessaggio("52960000", "", "famiglia", "corpo"), dalMessaggio("52950000", "", "famiglia", "oggetto")}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true, Identificativi: []db.IdentificativoThread{{Codice: "52950000"}}})
	nuovo := trova(t, c, "52960000").Stato
	if nuovo.Situazione != SituazioneNuovo || len(nuovo.Tipi) != 3 || nuovo.Tipi[0] != db.TipoComponenteFinito ||
		nuovo.Tipi[1] != db.TipoComponenteSottoassieme || nuovo.Tipi[2] != db.TipoComponenteSciolto {
		t.Errorf("nuovo: %s %v", nuovo.Situazione, nuovo.Tipi)
	}
	ric := trova(t, c, "52950000").Stato
	if ric.Situazione != SituazioneRichiesta || !ric.Identificativo || len(ric.Tipi) != 1 || ric.Tipi[0] != db.TipoComponenteFinito {
		t.Errorf("codice della richiesta: %s %v", ric.Situazione, ric.Tipi)
	}
}

// Con la BOM congelata la situazione resta quella e si mostra, ma nessun gesto la cambia da qui.
func TestConLaBomCongelataNessunGestoCheLaCambia(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{dalMessaggio("52960000", "", "famiglia", "corpo"), dalMessaggio("52950000", "", "famiglia", "oggetto")}
	c := Unisci(righe, ContestoCodici{HaFamiglie: true, Bloccata: 2, Identificativi: []db.IdentificativoThread{{Codice: "52950000"}}})
	if c.Bloccata != 2 {
		t.Errorf("Bloccata = %d", c.Bloccata)
	}
	for _, k := range append(c.Prodotto, c.Altri...) {
		if len(k.Stato.Tipi) != 0 || k.Stato.Bloccata != 2 {
			t.Errorf("%s: tipi %v, bloccata %d", k.Codice, k.Stato.Tipi, k.Stato.Bloccata)
		}
	}
	if s := trova(t, c, "52960000").Stato.Situazione; s != SituazioneNuovo {
		t.Errorf("la situazione si mostra anche a BOM congelata: %s", s)
	}
}

// Stesse righe in un altro ordine, stesso risultato: Unisci e' pura.
func TestUnisciNonDipendeDallOrdineDelleRighe(t *testing.T) {
	righe := []db.ListCodiciCandidatiThreadRow{
		dalMessaggio("52922757", "", "famiglia", "oggetto"), dalloStep("52922757", "B", "famiglia", "a.stp"),
		dalMessaggio("20260908", "", "generico", "corpo"), dalDocumento("52920517", "", "nome_file", "52920517.pdf", "disegno_2d", 70),
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
