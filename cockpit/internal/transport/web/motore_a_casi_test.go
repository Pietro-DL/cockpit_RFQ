package web

// L1 — Giro 5, A1d (QFE-01; piano di A1d frontend §8; RAC-04, RAC-05, RAC-08, RAC-20, RAC-27): i diciassette casi ACME
// della Distinta nuova (e1…e5, e8…e19). Il costruttore li fa nei tipi veri (valutazione, ancoraggio, fotorfq, confronto,
// motorea, db) e con MA_CASI_AGGIORNA=1 scrive testdata/motore_a/eN_<nome>.json (rientro di 2 spazi, LF, l'ordine dei
// campi dei tipi); senza la variabile li confronta byte per byte con quelli scritti. La lettura è stretta a ogni livello
// (DisallowUnknownFields) ed è una sola, leggiCasoMotoreA, che usano anche le prove di resa (QFE-02) e del backend (e3).
//
// Ogni caso ha la pagina (paginaMotoreA: testa, esito, confronto, errore, calcolato) e la fotografia minima del thread da
// cui nuovoIndiceMotoreA costruisce l'indice (RAC-08): esito.ancoraggi.strutture, esito.file, esito.associazioni,
// confronto.file e, nella foto, gli allegati con lo sha, i componenti, le relazioni, i documenti con i loro allegati, le
// proposte, le righe di componente_proposta e di relazione_proposta, le rimozioni aperte, le deroghe. Un Rif si fa solo con
// ancoraggio.RifNodo e ancoraggio.RifComponente.
//
// Solo dati ACME inventati (regola dei privati; RAC-27): cliente «ACME S.p.A.», codici 712xxxx, riferimento
// «ACME-030P7120100», UUID 00000000-0000-4000-8000-0000000000xx con lo schema di §8.1 (01 thread, 02 cliente,
// 03 messaggio, 10-2f componenti, 30-4f allegati, 50-6f documenti, 70-8f proposte, 90-af righe di componente_proposta,
// b0 utente, c0-cf deroghe), sha256 = %064x del numero dell'allegato. Mai catturati da una RFQ vera, nemmeno ripuliti.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

// casoMotoreA: un caso ACME della pagina (piano del frontend §8.1). Scrive sta fuori da Pagina perché
// paginaMotoreA.Scrive ha json:"-"; leggiCasoMotoreA lo rimette nella pagina.
type casoMotoreA struct {
	Caso   infoCaso       `json:"_caso"`  // titolo, contratto (sigle), attese (frasi)
	Scrive bool           `json:"scrive"` // paginaMotoreA.Scrive ha json:"-": sta fuori
	Pagina paginaMotoreA  `json:"pagina"` // thread_id, vista, prodotto, testa, esito, confronto, errore, calcolato
	Foto   fotorfq.Thread `json:"foto"`   // la fotografia minima da cui nuovoIndiceMotoreA costruisce l'indice
}

// infoCaso: che cosa il caso prova (il titolo), da dove viene (le sigle del contratto e delle decisioni) e che cosa la
// pagina deve mostrare (le frasi del piano del frontend §8.2).
type infoCaso struct {
	Titolo    string   `json:"titolo"`
	Contratto string   `json:"contratto"`
	Attese    []string `json:"attese"`
}

// maCasoNominato: un caso con il nome del suo file (eN_<nome>, senza .json).
type maCasoNominato struct {
	Nome string
	Caso casoMotoreA
}

// La cartella dei casi e il loro schema (RAC-04: solo e*_*.json, non ricorsivo: il kit ha altri .json).
const (
	maCartellaCasi = "testdata/motore_a"
	maSchemaCasi   = "e*_*.json"
)

// maNomiCasi: i diciassette casi di §8.2, nell'ordine (niente e0, e6, e7a, e7b: D-PF-11).
var maNomiCasi = []string{
	"e1_albero_candidato", "e2_due_strutture", "e3_bom_di_lavoro", "e4_confermati_senza_step", "e5_copia_nas",
	"e8_calcolo_non_riuscito", "e9_thread_non_valutato", "e10_da_smistare", "e11_sola_lettura",
	"e12_prodotto_senza_componente", "e13_completa_copie_in_corso", "e14_nodo_spostato", "e15_corrente_stesso_tipo",
	"e16_descrizione_registrata", "e17_da_smistare_contesto", "e18_stesso_rif_due_candidate", "e19_voce_da_decidere",
}

// maCasiACME: i diciassette casi, costruiti ogni volta da capo (nessun dato condiviso fra un caso e l'altro).
func maCasiACME() []maCasoNominato {
	return []maCasoNominato{
		{"e1_albero_candidato", maCasoE1()},
		{"e2_due_strutture", maCasoE2()},
		{"e3_bom_di_lavoro", maCasoE3()},
		{"e4_confermati_senza_step", maCasoE4()},
		{"e5_copia_nas", maCasoE5()},
		{"e8_calcolo_non_riuscito", maCasoE8()},
		{"e9_thread_non_valutato", maCasoE9()},
		{"e10_da_smistare", maCasoE10()},
		{"e11_sola_lettura", maCasoE11()},
		{"e12_prodotto_senza_componente", maCasoE12()},
		{"e13_completa_copie_in_corso", maCasoE13()},
		{"e14_nodo_spostato", maCasoE14()},
		{"e15_corrente_stesso_tipo", maCasoE15()},
		{"e16_descrizione_registrata", maCasoE16()},
		{"e17_da_smistare_contesto", maCasoE17()},
		{"e18_stesso_rif_due_candidate", maCasoE18()},
		{"e19_voce_da_decidere", maCasoE19()},
	}
}

// ---- scrittura e lettura ----

// maCodificaCaso: il JSON di un caso, con 2 spazi di rientro, LF e un a capo in fondo; i campi nell'ordine dei tipi, le
// chiavi delle mappe in ordine (encoding/json). Niente escape di <, > e & (i testi restano leggibili).
func maCodificaCaso(c casoMotoreA) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// maLeggiCasoStretto: un caso dal suo JSON, con DisallowUnknownFields (vale a ogni livello) e niente dopo il valore. Il
// campo scrive torna in Pagina.Scrive; Pagina.Indice resta nil (lo costruisce chi rende, con nuovoIndiceMotoreA).
func maLeggiCasoStretto(b []byte) (*casoMotoreA, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var c casoMotoreA
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("dopo il caso c'è dell'altro (%v)", err)
	}
	c.Pagina.Scrive = c.Scrive
	return &c, nil
}

// leggiCasoMotoreA: il caso «nome» di testdata/motore_a, letto stretto. nome è il nome del file senza .json
// («e3_bom_di_lavoro») o solo la sigla («e3»), che deve trovare un file solo.
func leggiCasoMotoreA(t testing.TB, nome string) *casoMotoreA {
	t.Helper()
	percorso := filepath.Join(maCartellaCasi, nome+".json")
	if _, err := os.Stat(percorso); err != nil {
		trovati, _ := filepath.Glob(filepath.Join(maCartellaCasi, nome+"_*.json"))
		if len(trovati) != 1 {
			t.Fatalf("caso %q: %d file in %s (ne serve uno)", nome, len(trovati), maCartellaCasi)
		}
		percorso = trovati[0]
	}
	b, err := os.ReadFile(percorso)
	if err != nil {
		t.Fatalf("caso %q: %v", nome, err)
	}
	c, err := maLeggiCasoStretto(b)
	if err != nil {
		t.Fatalf("caso %s: lettura stretta: %v", percorso, err)
	}
	return c
}

// ---- gli aiuti ACME dei casi (maInt e maStr vengono da motore_a_indice_test.go, RAC-20) ----

// maU: l'UUID ACME «00000000-0000-4000-8000-0000000000xx», con xx le due cifre esadecimali dello schema di §8.1.
func maU(xx string) uuid.UUID { return uuid.MustParse("00000000-0000-4000-8000-0000000000" + xx) }

// maUp: lo stesso UUID, come puntatore nuovo.
func maUp(xx string) *uuid.UUID {
	u := maU(xx)
	return &u
}

// maShaA: lo sha256 inventato dell'allegato xx, %064x del suo numero (§8.1).
func maShaA(xx string) string { return strings.Repeat("0", 62) + xx }

// maRN: il Rif del nodo «chiave» nel file dell'allegato xx (ancoraggio.RifNodo, mai a mano).
func maRN(all, chiave string) string { return ancoraggio.RifNodo(maShaA(all), chiave) }

// maRC: il Rif «componente:<uuid>» del componente xx (ancoraggio.RifComponente).
func maRC(xx string) string { return ancoraggio.RifComponente(maU(xx)) }

// maTrova: il primo elemento che risponde sì, come puntatore dentro l'elenco; nil se non c'è.
func maTrova[T any](xs []T, ok func(*T) bool) *T {
	for i := range xs {
		if ok(&xs[i]) {
			return &xs[i]
		}
	}
	return nil
}

// I dati fissi della testa (contratto §2.2; RAC-26: un'etichetta di fase inventata, mai una descrizione del catalogo).
var (
	maPresa      = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	maInizioFase = time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)
)

const (
	maCliente      = "ACME S.p.A."
	maOggetto      = "Richiesta di prova ACME"
	maRiferimento  = "ACME-030P7120100"
	maConfermatoIl = "2026-09-30T10:00:00Z"
	maMotivoGrafo  = "completezza del grafo non dichiarata dal documento" // ancoraggio, struttura_file.go:60
)

// maFase: la fase registrata dei casi (FATTIBILITA, «Fattibilità», semaforo verde).
func maFase() *faseRegistrata {
	return &faseRegistrata{Nome: string(db.FaseFATTIBILITA), Etichetta: "Fattibilità", Inizio: maInizioFase, GgInFase: 2,
		Semaforo: "verde"}
}

// ---- la scena: la foto del thread, l'esito e il confronto di un caso ----

type maScena struct {
	th fotorfq.Thread
	et valutazione.EsitoThread
	cf confronto.Esito
}

func maNuovaScena() *maScena {
	return &maScena{
		th: fotorfq.Thread{ID: maU("01"), ClienteID: maU("02"), Stato: string(db.StatoThreadAPERTA), Oggetto: maStr(maOggetto),
			Riferimento: maStr(maRiferimento), CreatoIl: maPresa},
		et: valutazione.EsitoThread{ThreadID: maU("01"), ClienteID: maU("02"), Valutato: true,
			Richiesta: ancoraggio.RichiestaValutata{ClienteID: maU("02"), Messaggi: []uuid.UUID{maU("03")},
				Stato: ancoraggio.StatoRichiestaValutata}},
		cf: confronto.Esito{VersioneConfronto: confronto.VersioneConfronto},
	}
}

// allegato: un file del messaggio 03 con il suo sha, e i suoi record nell'esito (file, associazione) e nel confronto
// (RAC-08). Di base: disponibile, candidato unico, figlio, nel perimetro, terminale, badge «uguale»; i casi cambiano
// quello che serve con all, file, assoc e badge.
func (s *maScena) allegato(xx, nome string) {
	est := strings.ToLower(nome[strings.LastIndex(nome, ".")+1:])
	s.th.Allegati = append(s.th.Allegati, fotorfq.Allegato{ID: maU(xx), MessaggioID: maU("03"), Indice: int16(len(s.th.Allegati)),
		NomeFile: nome, Estensione: maStr(est), Natura: string(db.NaturaAllegatoFile), Origine: string(db.OrigineAllegatoOutlook),
		Stato: string(db.StatoAllegatoAnalizzato), Sha256: maStr(maShaA(xx)), RicevutoIl: maPresa})
	s.et.File = append(s.et.File, valutazione.FileInterpretato{AllegatoID: maU(xx), Disponibilita: ancoraggio.DisponibilitaDisponibile,
		Disegno: valutazione.FormatoDi(est) != ""})
	s.et.Associazioni = append(s.et.Associazioni, valutazione.AssociazioneFile{AllegatoID: maU(xx),
		Associazione: ancoraggio.AssociazioneCandidatoUnico, Collocazione: ancoraggio.CollocazioneFiglio,
		Origine: ancoraggio.OrigineProposto, Terminale: true, Perimetro: valutazione.PerimetroDentro})
	s.cf.File = append(s.cf.File, confronto.EsitoFile{File: confronto.File{AllegatoID: maU(xx),
		Nuovo: confronto.Nuovo{Valutato: true}}, Badge: confronto.BadgeUguale})
}

func (s *maScena) all(xx string) *fotorfq.Allegato {
	return maDeve(maTrova(s.th.Allegati, func(a *fotorfq.Allegato) bool { return a.ID == maU(xx) }), "allegato", xx)
}

func (s *maScena) file(xx string) *valutazione.FileInterpretato {
	return maDeve(maTrova(s.et.File, func(f *valutazione.FileInterpretato) bool { return f.AllegatoID == maU(xx) }), "file", xx)
}

func (s *maScena) assoc(xx string) *valutazione.AssociazioneFile {
	return maDeve(maTrova(s.et.Associazioni, func(a *valutazione.AssociazioneFile) bool { return a.AllegatoID == maU(xx) }),
		"associazione", xx)
}

func (s *maScena) badge(xx string) *confronto.EsitoFile {
	return maDeve(maTrova(s.cf.File, func(f *confronto.EsitoFile) bool { return f.File.AllegatoID == maU(xx) }), "badge", xx)
}

func (s *maScena) comp(xx string) *fotorfq.Componente {
	return maDeve(maTrova(s.th.Componenti, func(c *fotorfq.Componente) bool { return c.ID == maU(xx) }), "componente", xx)
}

func (s *maScena) riga(xx string) *fotorfq.RigaComponenteProposta {
	return maDeve(maTrova(s.th.RigheComponenteProposta, func(r *fotorfq.RigaComponenteProposta) bool { return r.ID == maU(xx) }),
		"riga", xx)
}

// maDeve: un errore del costruttore (un riferimento a un record che il caso non ha) ferma tutto.
func maDeve[T any](p *T, che, xx string) *T {
	if p == nil {
		panic(fmt.Sprintf("costruttore dei casi: %s %s assente", che, xx))
	}
	return p
}

// componente: un componente confermato da una persona (utente b0).
func (s *maScena) componente(xx, codice string, tipo db.TipoComponente) {
	s.th.Componenti = append(s.th.Componenti, fotorfq.Componente{ID: maU(xx), Codice: codice, Tipo: string(tipo),
		Origine: string(db.OrigineComponenteManuale), ConfermatoDa: maU("b0"), CreatoIl: maPresa})
}

// relazione: un arco confermato padre → figlio.
func (s *maScena) relazione(padre, figlio string, qta int) {
	s.th.Relazioni = append(s.th.Relazioni, fotorfq.Relazione{PadreID: maU(padre), FiglioID: maU(figlio), Qta: qta,
		Origine: string(db.OrigineComponenteManuale), ConfermatoDa: maU("b0"), CreatoIl: maPresa})
}

// documento: un documento confermato del componente comp, nato dall'allegato all (nome, estensione e sha dal file).
func (s *maScena) documento(xx, comp string, tipo db.TipoDocumento, all string, nas db.StatoNas, rev *string) {
	a := s.all(all)
	s.th.Documenti = append(s.th.Documenti, fotorfq.DocumentoConfermato{ID: maU(xx), ThreadID: s.th.ID, ComponenteID: maUp(comp),
		Tipo: string(tipo), Codice: maStr(s.comp(comp).Codice), Rev: rev, NomeFile: a.NomeFile, Estensione: *a.Estensione,
		Sha256: *a.Sha256, StatoNas: string(nas), ConfermatoDa: maU("b0"), ConfermatoIl: maPresa, Allegati: []uuid.UUID{a.ID}})
}

// proposta: una riga di documento_proposta sull'allegato all; comp "" = senza componente.
func (s *maScena) proposta(xx, all string, tipo db.TipoDocumento, stato db.StatoProposta, comp string) {
	p := fotorfq.PropostaAttuale{ID: maU(xx), AllegatoID: maU(all), ThreadID: maUp("01"), Tipo: string(tipo),
		Fonte: string(db.FontePropostaNomeFile), Stato: string(stato)}
	if comp != "" {
		p.ComponenteID = maUp(comp)
	}
	s.th.Proposte = append(s.th.Proposte, p)
}

// rigaComp: una riga di componente_proposta del nodo «chiave» dello STEP all; comp "" = senza componente. Una riga non
// aperta è decisa da b0.
func (s *maScena) rigaComp(xx, all, chiave string, stato db.StatoProposta, comp string) {
	a := s.all(all)
	r := fotorfq.RigaComponenteProposta{ID: maU(xx), AllegatoID: a.ID, NomeFile: a.NomeFile, Sha256: *a.Sha256, Chiave: chiave,
		IDGrezzo: chiave, NomeGrezzo: "nodo " + chiave, Famiglia: "acme", Fonte: string(db.FontePropostaStep), Stato: string(stato)}
	if comp != "" {
		r.ComponenteID = maUp(comp)
	}
	if stato != db.StatoPropostaAperta {
		r.DecisoDa, r.DecisoIl = maUp("b0"), &maPresa
	}
	s.th.RigheComponenteProposta = append(s.th.RigheComponenteProposta, r)
}

// rigaRel: una riga di relazione_proposta dello STEP all, fra le chiavi padre e figlio.
func (s *maScena) rigaRel(all, padre, figlio string, qta int, stato db.StatoProposta) {
	s.th.RigheRelazioneProposta = append(s.th.RigheRelazioneProposta, fotorfq.RigaRelazioneProposta{AllegatoID: maU(all),
		NomeFile: s.all(all).NomeFile, PadreChiave: padre, FiglioChiave: figlio, Qta: qta, Stato: string(stato)})
}

// rimozione: una rimozione_proposta aperta dello STEP del documento step, sull'arco confermato padre → figlio.
func (s *maScena) rimozione(step, padre, figlio string, qta int) {
	s.th.RimozioniAperte = append(s.th.RimozioniAperte, fotorfq.RimozioneAperta{StepDocumentoID: maU(step), PadreID: maU(padre),
		FiglioID: maU(figlio), QtaWorking: qta})
}

// deroga: una deroga_fabbisogno del componente comp per il tipo di documento.
func (s *maScena) deroga(xx, comp string, tipo db.TipoDocumento) {
	s.th.Deroghe = append(s.th.Deroghe, fotorfq.DerogaFabbisogno{ID: maU(xx), ComponenteID: maU(comp), Tipo: string(tipo), Motivo: "Deroga di prova ACME",
		UtenteID: maU("b0"), CreataIl: maPresa})
}

// disegno: il 2D dell'allegato all; doc "" = un 2D proposto (nessun documento), altrimenti il documento corrente.
// L'anteprima è quella dell'allegato (un PDF con lo sha: disponibile).
func (s *maScena) disegno(all, doc, codice string, rev *string) valutazione.Disegno2D {
	a := s.all(all)
	d := valutazione.Disegno2D{AllegatoID: maUp(all), Sha256: *a.Sha256, NomeFile: a.NomeFile, Formato: valutazione.FormatoDi(*a.Estensione),
		Estensione: *a.Estensione, Validita: valutazione.ValiditaValido, Testo: true, Cartiglio: valutazione.CartiglioLetto,
		Codice: codice, Revisione: rev, FonteIdentita: valutazione.FonteIdentitaCartiglio, Provenienza: ancoraggio.OrigineProposto,
		CompatibilitaCodice: motorea.CompatibilitaUguale, CompatibilitaRevisione: motorea.CompatibilitaNonDeterminabile,
		ConfrontatoCon: valutazione.ConfrontatoConProposto,
		Anteprima: valutazione.RiferimentoAnteprima{Sha256: *a.Sha256, Formato: valutazione.FormatoDi(*a.Estensione), AllegatoID: maUp(all),
			Disponibile: valutazione.FormatoDi(*a.Estensione) == valutazione.Formato2DPDF},
		RevisioneProposta: valutazione.RevisioneProposta{Valore: rev, Fonte: ancoraggio.FonteEvidenzaCartiglio},
		StatoRevisione:    valutazione.StatoRevisione2DProposta}
	if rev == nil {
		d.RevisioneProposta.Fonte, d.StatoRevisione = valutazione.FontePropostaAssente, valutazione.StatoRevisione2DAssente
	}
	if doc != "" {
		d.DocumentoID, d.Anteprima.DocumentoID = maUp(doc), maUp(doc)
		d.Provenienza, d.Corrente, d.ConfermatoIl = ancoraggio.OrigineConfermato, true, &maPresa
		d.ConfrontatoCon = valutazione.ConfrontatoConDeciso
		if rev != nil {
			d.Registrata, d.RevProvenienza = rev, ancoraggio.RevProvenienzaNonRegistrata
			d.CompatibilitaRevisione, d.StatoRevisione = motorea.CompatibilitaUguale, valutazione.StatoRevisione2DRegistrata
		}
	}
	return d
}

// ---- i pezzi dell'esito ----

// maProdotto: il prodotto target del componente xx, con l'identità confermata; il resto lo scrive il caso.
func maProdotto(xx, codice string) valutazione.ProdottoValutato {
	return valutazione.ProdottoValutato{Rif: maRC(xx), Autorita: ancoraggio.AutoritaConfermata, ComponenteID: maUp(xx),
		CodiceRichiesto: codice, Base: motorea.BaseLetta{Originale: codice, Normalizzata: codice, Completa: true},
		Identita: valutazione.IdentitaConfermata, Struttura: ancoraggio.StatoStrutturaNessuna,
		Smistamento: valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoVerificato, Calcolata: true},
		Stato:       valutazione.ProdottoNonPronto}
}

// maNodoF: un nodo del file dell'allegato all (Rif = RifNodo dello sha del file e della chiave), leggibile, con la
// catena del codice proposta dallo STEP e i padri del file.
func maNodoF(all, chiave, codice string, padri ...valutazione.Parentela) valutazione.NodoBOM {
	return valutazione.NodoBOM{Nodo: ancoraggio.NodoProposto{Rif: maRN(all, chiave), AllegatoID: maU(all), EntitaID: chiave,
		Codice: ancoraggio.CatenaCodice{Grezzo: ancoraggio.ValoreGrezzo{Testo: codice, Campo: ancoraggio.CampoGrezzoID}, Lettura: codice,
			Identita: ancoraggio.IdentitaNodo{Base: codice, Provenienza: ancoraggio.ProvenienzaNodoSTEP,
				StatoRevisione: ancoraggio.StatoRevisioneAssente}, Proposto: codice},
		Leggibile: true}, Parentela: padri}
}

// maPadre: un padre della Parentela, con la quantità (dello STEP, o della relazione per un confermato senza STEP).
func maPadre(rif string, q int, decisa bool) valutazione.Parentela {
	return valutazione.Parentela{Padre: rif, Quantita: maInt(q), Decisa: decisa}
}

// maRadice: la radice del file (la provenienza della catena).
func maRadice(n *valutazione.NodoBOM) {
	n.Nodo.Codice.Identita.Provenienza = ancoraggio.ProvenienzaRadiceSTEP
}

// maProponi: il tipo proposto di un nodo senza decisione (EB7-2 A), con la riga aperta se c'è (riga "" = nessuna) e la
// classificazione di un nodo senza tipo deciso.
func (s *maScena) maProponi(n *valutazione.NodoBOM, tipo db.TipoComponente, origine valutazione.OrigineTipo, riga string) {
	n.Tipo, n.OrigineTipo = string(tipo), origine
	n.Classificazione = maClsProposta("")
	if riga != "" {
		r := s.riga(riga)
		n.Nodo.RigaLegacy = &ancoraggio.RigaPropostaLegacy{ID: r.ID, AllegatoID: r.AllegatoID, Sha256: r.Sha256, Chiave: r.Chiave,
			Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto}
	}
}

// maDecidi: il nodo è il componente xx (la Decisione per UUID e il codice confermato accanto), con la riga decisa se c'è
// (riga "" = nessuna) e il tipo e la classificazione del componente.
func (s *maScena) maDecidi(n *valutazione.NodoBOM, xx string, delProdotto bool, riga string) {
	c := s.comp(xx)
	n.Nodo.Decisione = &ancoraggio.ComponenteDeciso{ComponenteID: c.ID, Codice: c.Codice, Autorita: ancoraggio.AutoritaConfermata,
		Origine: ancoraggio.OrigineConfermato, Rev: c.Rev}
	n.Nodo.Codice.Confermato = &ancoraggio.CodiceDeciso{ComponenteID: maUp(xx), Codice: c.Codice, Rev: c.Rev,
		Origine: ancoraggio.OrigineConfermato, RevProvenienza: ancoraggio.RevProvenienzaNonRegistrata}
	n.Tipo, n.OrigineTipo = c.Tipo, valutazione.OrigineTipoConfermato
	n.Classificazione = maClsTipo(db.TipoComponente(c.Tipo), delProdotto)
	if riga != "" {
		r := s.riga(riga)
		n.Nodo.RigaDecisa = &ancoraggio.RigaDecisaLegacy{ID: r.ID, AllegatoID: r.AllegatoID, Sha256: r.Sha256, Chiave: r.Chiave,
			Stato: r.Stato, ComponenteID: r.ComponenteID, DecisoDa: r.DecisoDa}
		n.Nodo.DecisoDaPersona = true
	}
}

// maSenzaSTEP: un componente confermato che nessun nodo della BOM di lavoro rappresenta (EB7-1 B): Rif
// «componente:<uuid>», senza file, i padri dalle relazioni confermate.
func (s *maScena) maSenzaSTEP(xx string, delProdotto bool, padri ...valutazione.Parentela) valutazione.NodoBOM {
	c := s.comp(xx)
	n := valutazione.NodoBOM{Nodo: ancoraggio.NodoProposto{Rif: maRC(xx), SenzaFile: true,
		Codice: ancoraggio.CatenaCodice{Proposto: c.Codice}}, Parentela: padri}
	s.maDecidi(&n, xx, delProdotto, "")
	return n
}

// maClsProposta: la classificazione di un nodo senza tipo deciso (non determinata, proposta), con la categoria della
// grammatica accanto se c'è («minuteria proposta»: non esenta mai).
func maClsProposta(proposta string) valutazione.Classificazione {
	return valutazione.Classificazione{Ruolo: valutazione.RuoloComponente, Categoria: valutazione.CategoriaNonDeterminata,
		Origine: valutazione.OrigineCategoriaProposta, Motivo: valutazione.MotivoCategoriaTipoNonDeterminato, Proposta: proposta}
}

// maClsTipo: la classificazione dal tipo confermato (contratto §2.5): fabbricato derivato dal tipo, commerciale
// confermato; prodotto per il finito e per il componente del target.
func maClsTipo(tipo db.TipoComponente, delProdotto bool) valutazione.Classificazione {
	c := valutazione.Classificazione{Ruolo: valutazione.RuoloComponente, Categoria: valutazione.CategoriaFabbricato,
		Origine: valutazione.OrigineCategoriaDerivata}
	if tipo == db.TipoComponenteFinito || delProdotto {
		c.Ruolo = valutazione.RuoloProdotto
	}
	if tipo == db.TipoComponenteCommerciale {
		c.Categoria, c.Origine, c.Confermata = valutazione.CategoriaCommerciale, valutazione.OrigineCategoriaConfermata, true
	}
	return c
}

// maAsse: un asse della verifica della BOM.
func maAsse(stato valutazione.StatoVerificaAsse, motivo string, daDecidere int) valutazione.VerificaAsse {
	return valutazione.VerificaAsse{Stato: stato, Motivo: motivo, DaDecidere: daDecidere}
}

// maBOMSenzaFonte: la BOM senza fonte confermata: i due assi non verificabili, con il motivo.
func maBOMSenzaFonte(motivo string) valutazione.VerificaBOM {
	return valutazione.VerificaBOM{Nomenclatura: maAsse(valutazione.StatoAsseNonVerificabile, motivo, 0),
		Gerarchia: maAsse(valutazione.StatoAsseNonVerificabile, motivo, 0)}
}

// maFonteConfermata: la fonte confermata dal gesto 3 sul documento doc (lo STEP dell'allegato all), con la radice
// registrata (la chiave).
func (s *maScena) maFonteConfermata(doc, all, radice string) valutazione.FonteProdotto {
	return valutazione.FonteProdotto{Stato: valutazione.FonteConfermata, Motivo: valutazione.MotivoFonteEstrazioneRiuscita,
		EsitoVista: "presente_analizzato", Calcolata: true, Estrazione: ancoraggio.EstrazioneRiuscita,
		Riferimento: &ancoraggio.RiferimentoFonte{Tipo: ancoraggio.TipoRiferimentoStep, DocumentoID: maU(doc), Sha256: *s.all(all).Sha256,
			AllegatoID: maUp(all), Radice: radice, Ruolo: "radice", Forma: ancoraggio.FormaRiferimentoSmistamento,
			ConfermatoDa: maUp("b0"), ConfermatoIl: maConfermatoIl}}
}

// maCandidato: un documento candidato della fonte (M1).
func maCandidato(origine ancoraggio.OrigineCandidato, all, doc string, radici []string, compat motorea.Compatibilita,
	estr ancoraggio.EsitoEstrazione, grafo bool) ancoraggio.DocumentoCandidato {
	c := ancoraggio.DocumentoCandidato{Origine: origine, AllegatoID: maUp(all), Sha256: maShaA(all), Radici: radici,
		Compatibilita: compat, Estrazione: estr, GrafoCompleto: grafo}
	if len(radici) > 0 {
		c.RadiceCompatibile = radici[0]
	}
	if doc != "" {
		c.DocumentoID = maUp(doc)
	}
	if !grafo {
		c.MotivoGrafo = maMotivoGrafo
	}
	return c
}

// maCandidata: una struttura candidata (EB7-1 B) dell'allegato all sotto la radice (chiave).
func maCandidata(all, radice string, delFile bool, compat motorea.Compatibilita, grafo bool,
	nodi ...valutazione.NodoBOM) valutazione.StrutturaCandidata {
	sc := valutazione.StrutturaCandidata{AllegatoID: maU(all), Sha256: maShaA(all), Radice: maRN(all, radice), RadiceDelFile: delFile,
		Compatibilita: compat, GrafoCompleto: grafo, Nodi: nodi}
	if !grafo {
		sc.MotivoGrafo = maMotivoGrafo
	}
	return sc
}

// maVoce: una voce certa (componente, tipo) bloccante, calcolata in Go per il 2D e dalla vista per gli altri tipi.
func (s *maScena) maVoce(comp string, tipo db.TipoDocumento, delProdotto bool, esito valutazione.EsitoFabbisogno) valutazione.VoceFabbisogno {
	c := s.comp(comp)
	v := valutazione.VoceFabbisogno{ComponenteID: c.ID, Codice: c.Codice, TipoComponente: c.Tipo, DelProdotto: delProdotto,
		TipoDocumento: string(tipo), Bloccante: true, Esito: esito, CalcolataDa: valutazione.CalcolataDaVista,
		Categoria: maClsTipo(db.TipoComponente(c.Tipo), delProdotto).Categoria}
	if tipo == db.TipoDocumentoDisegno2d {
		v.CalcolataDa, v.Invariante = valutazione.CalcolataDaGo, true
	}
	if esito == valutazione.EsitoManca {
		v.Motivo = valutazione.MotivoFabbisognoNessunDocumento
	}
	return v
}

// maConDocumento: la voce è decisa dal documento doc, con lo stato del NAS della foto (EB7-4 A).
func (s *maScena) maConDocumento(v *valutazione.VoceFabbisogno, doc string) {
	d := maDeve(maTrova(s.th.Documenti, func(d *fotorfq.DocumentoConfermato) bool { return d.ID == maU(doc) }), "documento", doc)
	v.DocumentoID, v.StatoNas = maUp(doc), d.StatoNas
}

// maPrevisto: un fabbisogno previsto (il 2D di un nodo proposto, o di un deciso fuori dal perimetro).
func maPrevisto(nodo, codice string, disegni *valutazione.GruppoDisegni2D, motivo string) valutazione.FabbisognoPrevisto {
	return valutazione.FabbisognoPrevisto{Nodo: nodo, CodiceProposto: codice, TipoDocumento: string(db.TipoDocumentoDisegno2d),
		Disegni: disegni, Motivo: motivo}
}

// maOrigine: un'origine come puntatore (NodoBOM.Associazione).
func maOrigine(o ancoraggio.OrigineDato) *ancoraggio.OrigineDato { return &o }

// ---- la chiusura del caso ----

// strutturePer: le strutture di ancoraggio del prodotto (RAC-08) che l'indice legge: la BOM di lavoro (dalla fonte
// confermata, se il prodotto ha Nodi) e una per ogni struttura candidata, con gli archi dei padri dei nodi. Un arco che
// la Parentela dice deciso porta accanto la relazione confermata fra i componenti dei due nodi (la radice della BOM di
// lavoro vale il componente del target: T-B4-23, T-A1d-22); se la relazione non c'è, il costruttore si ferma.
func (s *maScena) strutturePer(p *valutazione.ProdottoValutato) []ancoraggio.StrutturaProdotto {
	var out []ancoraggio.StrutturaProdotto
	if len(p.Nodi) > 0 {
		r := maDeve(p.Fonte.Riferimento, "riferimento della fonte di", p.Rif)
		radice := ancoraggio.RifNodo(r.Sha256, r.Radice)
		out = append(out, ancoraggio.StrutturaProdotto{Target: p.Rif, AllegatoID: *r.AllegatoID, Sha256: r.Sha256, Radice: radice,
			RadiceDelFile: true, Compatibilita: motorea.CompatibilitaUguale, Stato: ancoraggio.StatoBOMDiLavoroProposta,
			Archi: s.archi(p, p.Nodi, radice), GrafoCompleto: true})
	}
	for _, sc := range p.StruttureCandidate {
		out = append(out, ancoraggio.StrutturaProdotto{Target: p.Rif, AllegatoID: sc.AllegatoID, Sha256: sc.Sha256, Radice: sc.Radice,
			RadiceDelFile: sc.RadiceDelFile, Compatibilita: sc.Compatibilita, Stato: ancoraggio.StatoStrutturaCandidata,
			Archi: s.archi(p, sc.Nodi, ""), GrafoCompleto: sc.GrafoCompleto, MotivoGrafo: sc.MotivoGrafo})
	}
	return out
}

// archi: gli archi della struttura dai padri dei suoi nodi, in ordine di (padre, figlio).
func (s *maScena) archi(p *valutazione.ProdottoValutato, nodi []valutazione.NodoBOM, radiceBOM string) []ancoraggio.ArcoProposto {
	componenteDi := func(rif string) *uuid.UUID {
		for i := range nodi {
			if nodi[i].Nodo.Rif != rif {
				continue
			}
			if nodi[i].Nodo.Decisione != nil {
				return &nodi[i].Nodo.Decisione.ComponenteID
			}
			if rif == radiceBOM {
				return p.ComponenteID
			}
		}
		return nil
	}
	var out []ancoraggio.ArcoProposto
	for _, n := range nodi {
		for _, par := range n.Parentela {
			a := ancoraggio.ArcoProposto{Padre: par.Padre, Figlio: n.Nodo.Rif, Quantita: maInt(*par.Quantita)}
			if par.Decisa {
				pc, fc := componenteDi(par.Padre), componenteDi(n.Nodo.Rif)
				if pc == nil || fc == nil {
					panic("costruttore dei casi: arco deciso senza i due componenti: " + par.Padre + " > " + n.Nodo.Rif)
				}
				rel := maDeve(maTrova(s.th.Relazioni, func(r *fotorfq.Relazione) bool { return r.PadreID == *pc && r.FiglioID == *fc }),
					"relazione", pc.String()+">"+fc.String())
				a.Decisione = &ancoraggio.ArcoPercorso{Padre: ancoraggio.RifComponente(*pc), Figlio: ancoraggio.RifComponente(*fc),
					Quantita: maInt(rel.Qta), Origine: ancoraggio.OrigineArcoConfermato}
			}
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Padre != out[j].Padre {
			return out[i].Padre < out[j].Padre
		}
		return out[i].Figlio < out[j].Figlio
	})
	return out
}

// chiudi: il caso dalla scena. Mette le strutture di ancoraggio dei prodotti; ordina file, associazioni e badge per
// allegato (l'ordine dell'esito e del confronto); conta i badge (tutti e cinque); compone il fascicolo dai prodotti
// (numeri, pronti, bloccati, congelabile) e dagli orfani (gli avvisi nel formato di F0-18, valutazione/pertinenza.go:
// «orfano:<allegato_id>», con «:prodotti_contesto:<rif>|<rif>»); la testa dalla foto; il prodotto scelto è il primo
// (prodottoScelto senza una richiesta).
func (s *maScena) chiudi(info infoCaso, scrive bool) casoMotoreA {
	s.et.Ancoraggi.Strutture = nil
	for i := range s.et.ProdottiValutati {
		s.et.Ancoraggi.Strutture = append(s.et.Ancoraggi.Strutture, s.strutturePer(&s.et.ProdottiValutati[i])...)
	}
	st := s.et.Ancoraggi.Strutture
	sort.SliceStable(st, func(i, j int) bool {
		a, b := st[i], st[j]
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.Sha256 != b.Sha256 {
			return a.Sha256 < b.Sha256
		}
		if a.AllegatoID != b.AllegatoID {
			return a.AllegatoID.String() < b.AllegatoID.String()
		}
		return a.Radice < b.Radice
	})
	sort.SliceStable(s.et.File, func(i, j int) bool { return s.et.File[i].AllegatoID.String() < s.et.File[j].AllegatoID.String() })
	sort.SliceStable(s.et.Associazioni, func(i, j int) bool {
		return s.et.Associazioni[i].AllegatoID.String() < s.et.Associazioni[j].AllegatoID.String()
	})
	sort.SliceStable(s.cf.File, func(i, j int) bool {
		return s.cf.File[i].File.AllegatoID.String() < s.cf.File[j].File.AllegatoID.String()
	})
	s.cf.Conteggi = map[confronto.Badge]int{confronto.BadgeUguale: 0, confronto.BadgeNuovoAncoraggio: 0, confronto.BadgeDiverso: 0,
		confronto.BadgeRegressioneSuConfermata: 0, confronto.BadgeFuoriRichiesta: 0}
	for _, f := range s.cf.File {
		s.cf.Conteggi[f.Badge]++
	}

	f := &s.et.Fascicolo
	f.NumeroTarget, f.NumeroVerificati, f.Pronti, f.Bloccati = len(s.et.ProdottiValutati), 0, nil, nil
	for _, p := range s.et.ProdottiValutati {
		if p.Verificato {
			f.NumeroVerificati++
			f.Pronti = append(f.Pronti, p.Rif)
		} else {
			f.Bloccati = append(f.Bloccati, valutazione.ProdottoBloccato{Rif: p.Rif, Motivi: p.Motivi})
		}
	}
	f.Congelabile, f.MotivoNonCongelabile, f.MotivoNonCongelato = false, "", ""
	switch {
	case !s.et.Valutato:
		f.MotivoNonCongelabile = valutazione.NonCongelabileThreadNonValutato
	case f.NumeroTarget == 0:
		f.MotivoNonCongelabile = valutazione.NonCongelabileNessunTarget
	case len(f.Bloccati) > 0:
		f.MotivoNonCongelabile = valutazione.NonCongelabileProdottiNonPronti
	default:
		f.Congelabile, f.MotivoNonCongelato = true, valutazione.NonCongelatoGestoNonRegistrato
	}
	f.Calcolato, f.FaseThread = true, s.th.Stato
	f.Orfani, f.Avvisi = 0, nil
	for _, d := range s.et.DaSmistare {
		if !d.Orfano {
			continue
		}
		f.Orfani++
		a := "orfano:" + d.AllegatoID.String()
		if len(d.ProdottiContesto) > 0 {
			a += ":prodotti_contesto:" + strings.Join(d.ProdottiContesto, "|")
		}
		f.Avvisi = append(f.Avvisi, a)
	}

	et, cf := s.et, s.cf
	pg := paginaMotoreA{ThreadID: s.th.ID, Vista: "distinta", Esito: &et, Confronto: &cf, Scrive: scrive, Calcolato: maPresa,
		Testa: testaMotoreA{Cliente: maCliente, Oggetto: *s.th.Oggetto, RiferimentoCliente: *s.th.Riferimento, StatoThread: s.th.Stato,
			Fase: maFase()}}
	if len(et.ProdottiValutati) > 0 {
		pg.Prodotto = et.ProdottiValutati[0].Rif
	}
	return casoMotoreA{Caso: info, Scrive: scrive, Pagina: pg, Foto: s.th}
}

// ---- i casi (piano del frontend §8.2; codici di RAC-27: ACME-100 → 7120100, ACME-110 → 7120110, …, la vite
// ISO4762-M6x20 → 7129020, ACME-T999 → 7129999) ----

// e1: una struttura candidata dal file, nessuna conferma (E1, EB7-1, EB7-2).
func maCasoE1() casoMotoreA {
	s := maNuovaScena()
	s.allegato("30", "7120100_C.stp")
	s.allegato("32", "7120110.pdf")
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.rigaComp("90", "30", "#10", db.StatoPropostaAperta, "")
	s.riga("90").TipoProposto = maStr(string(db.TipoComponenteSottoassieme))

	n10 := maNodoF("30", "#10", "7120100")
	maRadice(&n10)
	s.maProponi(&n10, db.TipoComponenteSottoassieme, valutazione.OrigineTipoPropostoDallaRiga, "90")
	d32 := s.disegno("32", "", "7120110", nil)
	n20 := maNodoF("30", "#20", "7120110", maPadre(maRN("30", "#10"), 2, false))
	s.maProponi(&n20, db.TipoComponenteSciolto, valutazione.OrigineTipoPropostoDallaRegola, "")
	n20.Disegni = valutazione.GruppoDisegni2D{Primario: &d32, RegolaPrimario: valutazione.RegolaPrimarioFormato}
	n20.Associazione = maOrigine(ancoraggio.OrigineProposto)
	n30 := maNodoF("30", "#30", "7129020", maPadre(maRN("30", "#10"), 8, false))
	s.maProponi(&n30, db.TipoComponenteSciolto, valutazione.OrigineTipoPropostoDallaRegola, "")
	n30.Classificazione.Proposta = valutazione.CategoriaMinuteria

	p := maProdotto("10", "7120100")
	p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteDocumentoCandidato,
		EsitoVista: "mancante", Calcolata: true, Candidati: []ancoraggio.DocumentoCandidato{
			maCandidato(ancoraggio.CandidatoDaMotoreA, "30", "", []string{"#10"}, motorea.CompatibilitaUguale, ancoraggio.EstrazioneRiuscita, true)}}
	p.Struttura = ancoraggio.StatoStrutturaCandidata
	p.BOM = maBOMSenzaFonte(valutazione.MotivoAsseFonteNonConfermata)
	p.StruttureCandidate = []valutazione.StrutturaCandidata{
		maCandidata("30", "#10", true, motorea.CompatibilitaUguale, true, n10, n20, n30)}
	p.ConfermatiSenzaSTEP = []valutazione.NodoBOM{s.maSenzaSTEP("10", true)}
	v10 := s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca)
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci: []valutazione.VoceFabbisogno{v10},
		Previsti: []valutazione.FabbisognoPrevisto{
			maPrevisto(maRN("30", "#20"), "7120110", &valutazione.GruppoDisegni2D{Primario: &d32}, valutazione.MotivoPrevistoNodoProposto),
			maPrevisto(maRN("30", "#30"), "7129020", nil, valutazione.MotivoPrevistoNodoProposto)},
		MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
	p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true,
		FileNonTerminali: []uuid.UUID{maU("32")}, Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoAssociazioneNonConfermata}}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoNomenclatura,
		valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoSmistamento, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}

	s.assoc("30").Collocazione, s.assoc("30").Proposta = ancoraggio.CollocazioneRadice, []string{p.Rif}
	s.assoc("32").Proposta, s.assoc("32").Terminale = []string{p.Rif}, false
	s.badge("30").Badge, s.badge("32").Badge = confronto.BadgeNuovoAncoraggio, confronto.BadgeNuovoAncoraggio

	return s.chiudi(infoCaso{Titolo: "E1: una struttura candidata dal file, nessuna conferma",
		Contratto: "A1d §4 E1; EB7-1; EB7-2", Attese: []string{
			"schede «cand» (7120100_C.stp, radice #10) e «senza»: la prima scheda visibile, nessun segno di scelta",
			"nel pannello della candidata la frase «proposta dal file, non è la BOM»",
			"«Autorizza» spento con il motivo: manca il documento confermato del componente con quel file (P-14)",
			"la miniatura di #20 (7120110.pdf): l'anteprima è disponibile",
			"#30 (7129020) è «minuteria proposta», senza esenzione dal 2D",
			"il segnaposto «BOM di lavoro: non ancora»; nessun trascinamento nella candidata",
		}}, true)
}

// e2: due strutture candidate e un terzo candidato della fonte (E2, D6).
func maCasoE2() casoMotoreA {
	s := maNuovaScena()
	s.allegato("30", "7120100_C.stp")
	s.allegato("31", "7120100_v2.stp")
	s.allegato("33", "7120100_D.stp")
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.documento("50", "10", db.TipoDocumentoCad3d, "30", db.StatoNasScritto, nil)
	s.proposta("70", "33", db.TipoDocumentoCad3d, db.StatoPropostaAperta, "10")
	s.rigaComp("90", "30", "#10", db.StatoPropostaAperta, "")
	s.rigaComp("91", "31", "#1", db.StatoPropostaAperta, "")

	a10 := maNodoF("30", "#10", "7120100")
	maRadice(&a10)
	s.maProponi(&a10, db.TipoComponenteSottoassieme, valutazione.OrigineTipoPropostoDallaRegola, "90")
	a20 := maNodoF("30", "#20", "7120110", maPadre(maRN("30", "#10"), 2, false))
	s.maProponi(&a20, db.TipoComponenteSottoassieme, valutazione.OrigineTipoPropostoDallaRegola, "")
	a30 := maNodoF("30", "#30", "7120120", maPadre(maRN("30", "#20"), 4, false))
	s.maProponi(&a30, db.TipoComponenteSciolto, valutazione.OrigineTipoPropostoDallaRegola, "")
	b1 := maNodoF("31", "#1", "7120100")
	maRadice(&b1)
	s.maProponi(&b1, db.TipoComponenteSottoassieme, valutazione.OrigineTipoPropostoDallaRegola, "91")
	b2 := maNodoF("31", "#2", "7120110", maPadre(maRN("31", "#1"), 1, false))
	s.maProponi(&b2, db.TipoComponenteSciolto, valutazione.OrigineTipoPropostoDallaRegola, "")

	p := maProdotto("10", "7120100")
	terzo := maCandidato(ancoraggio.CandidatoDaPropostaAperta, "33", "", nil, "", ancoraggio.EstrazioneNonAnalizzata, false)
	terzo.MotivoGrafo = ""
	p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteDaScegliere,
		EsitoVista: "da_scegliere", Calcolata: true, Candidati: []ancoraggio.DocumentoCandidato{
			maCandidato(ancoraggio.CandidatoDaDocumentoDelProdotto, "30", "50", []string{"#10"}, motorea.CompatibilitaUguale,
				ancoraggio.EstrazioneRiuscita, true),
			maCandidato(ancoraggio.CandidatoDaMotoreA, "31", "", []string{"#1"}, motorea.CompatibilitaParziale,
				ancoraggio.EstrazioneRiuscita, false),
			terzo}}
	p.Struttura = ancoraggio.StatoStrutturaCandidata
	p.BOM = maBOMSenzaFonte(valutazione.MotivoAsseFonteNonConfermata)
	p.StruttureCandidate = []valutazione.StrutturaCandidata{
		maCandidata("30", "#10", true, motorea.CompatibilitaUguale, true, a10, a20, a30),
		maCandidata("31", "#1", true, motorea.CompatibilitaParziale, false, b1, b2)}
	p.ConfermatiSenzaSTEP = []valutazione.NodoBOM{s.maSenzaSTEP("10", true)}
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci:            []valutazione.VoceFabbisogno{s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca)},
		MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoNomenclatura,
		valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}

	for _, x := range []string{"30", "31", "33"} {
		s.assoc(x).Collocazione, s.assoc(x).Proposta = ancoraggio.CollocazioneRadice, []string{p.Rif}
	}
	s.assoc("30").DocumentoID, s.assoc("30").Confermata, s.assoc("30").Origine = maUp("50"), maUp("10"), ancoraggio.OrigineConfermato
	s.badge("31").Badge, s.badge("33").Badge = confronto.BadgeNuovoAncoraggio, confronto.BadgeNuovoAncoraggio

	return s.chiudi(infoCaso{Titolo: "E2: due strutture candidate, un altro documento candidato alla fonte",
		Contratto: "A1d §4 E2; D6 di P7e", Attese: []string{
			"tre schede: A (7120100_C.stp, radice #10), B (7120100_v2.stp, radice #1), «senza»",
			"A: «Autorizza» acceso con documento = 00000000-0000-4000-8000-000000000050",
			"B: «Autorizza» spento con il motivo (nessun documento confermato con quel file); compatibile parziale, grafo incompleto",
			"«Altri documenti candidati alla fonte (1)»: 7120100_D.stp (proposta aperta)",
		}}, true)
}

// e3: la BOM di lavoro con nodi in tutti gli stati, un confermato fuori dallo STEP e una rimozione (E3).
func maCasoE3() casoMotoreA {
	s := maNuovaScena()
	s.allegato("30", "7120100_C.stp")
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.componente("14", "7120140", db.TipoComponenteSciolto)
	s.componente("16", "7129020", db.TipoComponenteCommerciale)
	s.componente("17", "7120170", db.TipoComponenteSciolto)
	s.documento("50", "10", db.TipoDocumentoCad3d, "30", db.StatoNasScritto, nil)
	s.comp("10").StepStrutturaleID = maUp("50")
	s.relazione("10", "14", 1)
	s.relazione("10", "17", 1)
	s.rigaComp("91", "30", "#20", db.StatoPropostaAperta, "")
	r91 := s.riga("91")
	r91.OrigineCodice, r91.Codice, r91.TipoProposto = maStr("operatore"), maStr("7120110-B"), maStr(string(db.TipoComponenteSottoassieme))
	s.rigaComp("92", "30", "#50", db.StatoPropostaScartata, "")
	s.rigaComp("94", "30", "#40", db.StatoPropostaConfermata, "14")
	s.rigaComp("96", "30", "#60", db.StatoPropostaConfermata, "16")
	s.rigaRel("30", "#10", "#20", 1, db.StatoPropostaAperta)
	s.rimozione("50", "10", "17", 1)

	radice := maRN("30", "#10")
	n10 := maNodoF("30", "#10", "7120100")
	maRadice(&n10)
	n10.Tipo, n10.OrigineTipo, n10.Classificazione = string(db.TipoComponenteFinito), valutazione.OrigineTipoConfermato,
		maClsTipo(db.TipoComponenteFinito, true)
	n10.Nodo.Codice.Confermato = &ancoraggio.CodiceDeciso{ComponenteID: maUp("10"), Codice: "7120100", Origine: ancoraggio.OrigineConfermato}
	n20 := maNodoF("30", "#20", "7120110", maPadre(radice, 1, false))
	s.maProponi(&n20, db.TipoComponenteSottoassieme, valutazione.OrigineTipoPropostoDallaRiga, "91")
	n20.Nodo.RigaLegacy.CodiceManuale = maStr("7120110-B")
	n20.Nodo.Codice.Manuale = &ancoraggio.CodiceDeciso{Codice: "7120110-B", Origine: ancoraggio.OrigineManuale}
	n40 := maNodoF("30", "#40", "7120140", maPadre(radice, 1, true), maPadre(maRN("30", "#20"), 2, false))
	s.maDecidi(&n40, "14", false, "94")
	n50 := maNodoF("30", "#50", "7120150", maPadre(radice, 1, false))
	s.maProponi(&n50, db.TipoComponenteSciolto, valutazione.OrigineTipoPropostoDallaRegola, "")
	r92 := s.riga("92")
	n50.Nodo.RigaDecisa = &ancoraggio.RigaDecisaLegacy{ID: r92.ID, AllegatoID: r92.AllegatoID, Sha256: r92.Sha256, Chiave: r92.Chiave,
		Stato: ancoraggio.StatoRigaScartata, DecisoDa: r92.DecisoDa}
	n50.Nodo.DecisoDaPersona = true
	n50.Nodo.AbbinamentoPerBase = &ancoraggio.AbbinamentoPerBase{Motivo: ancoraggio.MotivoNodoScartato}
	n60 := maNodoF("30", "#60", "7129020", maPadre(radice, 8, false))
	s.maDecidi(&n60, "16", false, "96")
	n60.Classificazione = valutazione.Classificazione{Ruolo: valutazione.RuoloComponente, Categoria: valutazione.CategoriaMinuteria,
		Origine: valutazione.OrigineCategoriaConfermata, Confermata: true}
	n70 := maNodoF("30", "#70", "7120180", maPadre(radice, 1, false))
	n70.Classificazione = maClsProposta("")

	p := maProdotto("10", "7120100")
	p.Fonte = s.maFonteConfermata("50", "30", "#10")
	p.Struttura = ancoraggio.StatoBOMDiLavoroProposta
	arco1017 := valutazione.RifArco(maRC("10"), maRC("17"))
	p.BOM = valutazione.VerificaBOM{
		Nomenclatura: maAsse(valutazione.StatoAsseDaVerificare, valutazione.MotivoAsseDaDecidere, 1),
		Gerarchia: valutazione.VerificaAsse{Stato: valutazione.StatoAsseConflitto, DaDecidere: 1, Conflitti: []string{arco1017},
			Motivo: valutazione.MotivoAsseConflitto},
		FonteRegistrata: true, RimozioniAperte: 1}
	p.Nodi = []valutazione.NodoBOM{n10, n20, n40, n50, n60, n70}
	p.ConfermatiSenzaSTEP = []valutazione.NodoBOM{s.maSenzaSTEP("17", false, maPadre(radice, 1, true))}
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci: []valutazione.VoceFabbisogno{
			s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca),
			s.maVoce("14", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca),
			s.maVoce("17", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca)},
		Previsti: []valutazione.FabbisognoPrevisto{
			maPrevisto(maRN("30", "#20"), "7120110-B", nil, valutazione.MotivoPrevistoNodoProposto),
			maPrevisto(maRN("30", "#70"), "7120180", nil, valutazione.MotivoPrevistoNodoProposto)},
		MotivoPerimetro: valutazione.MotivoPerimetroNodiDaDecidere}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoNomenclatura, valutazione.MotivoProdottoGerarchia,
		valutazione.MotivoProdottoDocumentazioneIncompleta, valutazione.MotivoProdottoConflitto}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}
	s.et.Conflitti = []valutazione.Conflitto{
		{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia, Rif: arco1017, Prodotto: p.Rif, Decisione: "qta 1",
			OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: valutazione.MotivoConflittoRimozione,
			Motivo: valutazione.MotivoConflittoRimozione, EvidenzaDecisione: valutazione.EvidenzaDecisione{
				Origine: ancoraggio.OrigineConfermato, Da: maUp("b0"), Il: &maPresa},
			EvidenzaProposta: valutazione.EvidenzaProposta{DocumentoID: maUp("50")}},
		{Tipo: valutazione.ConflittoCodice, Asse: valutazione.AsseNomenclatura, Rif: maRN("30", "#20"), Prodotto: p.Rif,
			Decisione: "7120110-B", OrigineDecisione: ancoraggio.OrigineManuale, Proposta: "7120110",
			Documentale: ancoraggio.RiconciliazioneDiscordante, Motivo: ancoraggio.ParteDiscordanzaCodice,
			EvidenzaDecisione: valutazione.EvidenzaDecisione{Origine: ancoraggio.OrigineManuale},
			EvidenzaProposta: valutazione.EvidenzaProposta{AllegatoID: maUp("30"),
				Evidenza:    ancoraggio.EvidenzaVista{Fonte: ancoraggio.FonteEvidenzaStepEntita, Valore: "7120110"},
				Riferimenti: []string{maRN("30", "#20")}}},
	}

	a := s.assoc("30")
	a.Collocazione, a.Proposta, a.DocumentoID, a.Confermata, a.Origine = ancoraggio.CollocazioneRadice, []string{p.Rif}, maUp("50"),
		maUp("10"), ancoraggio.OrigineConfermato

	return s.chiudi(infoCaso{Titolo: "E3: la BOM di lavoro, con nodi aperti, decisi, scartati e un confermato fuori dallo STEP",
		Contratto: "A1d §4 E3", Attese: []string{
			"schede «bom» e «senza»; nella «bom» 7120170 sotto #10 con la linea piena (X-03)",
			"linea piena #10 → #40, tratteggiata #20 → #40 e #10 → #20",
			"«⧉ 2 padri nel file» su #40",
			"«Accetta il legame» e «Scarta il legame» nella Posizione di #20",
			"i gesti della rimozione in R7 (conflitto arco/rimozione dello STEP 50, 7120100 → 7120170)",
			"il nodo scartato #50 con «Riapri»",
			"#70: «tipo non determinato»",
			"trascinamento su #40 con data-ma-da = 00000000-0000-4000-8000-000000000010",
		}}, true)
}

// e4: solo confermati, nessuno STEP (E4).
func maCasoE4() casoMotoreA {
	s := maNuovaScena()
	s.componente("20", "7120200", db.TipoComponenteFinito)
	s.componente("21", "7120210", db.TipoComponenteCommerciale)
	s.comp("21").Rev = maStr("B")
	s.relazione("20", "21", 4)

	p := maProdotto("20", "7120200")
	p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteAssente, Motivo: valutazione.MotivoFonteNessunaFonte, EsitoVista: "mancante",
		Calcolata: true}
	p.BOM = maBOMSenzaFonte(valutazione.MotivoAsseNessunaStruttura)
	p.ConfermatiSenzaSTEP = []valutazione.NodoBOM{s.maSenzaSTEP("20", true), s.maSenzaSTEP("21", false, maPadre(maRC("20"), 4, true))}
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci: []valutazione.VoceFabbisogno{
			s.maVoce("20", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca),
			s.maVoce("21", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca)},
		MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoNomenclatura,
		valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}

	return s.chiudi(infoCaso{Titolo: "E4: confermati senza STEP", Contratto: "A1d §4 E4", Attese: []string{
		"sola scheda «senza»: 7120200 e sotto 7120210 rev B",
		"linea piena e «✓×4 confermata»",
		"7120210: «commerciale: non è un'esenzione dal 2D»",
		"trascinamento consentito nella scheda «senza»",
	}}, true)
}

// e5: le voci dei documenti con le copie sul NAS (E5, EB7-4).
func maCasoE5() casoMotoreA {
	s := maNuovaScena()
	s.allegato("34", "7120200_B.pdf")
	s.allegato("35", "7120200.dxf")
	s.allegato("36", "7120210_B.pdf")
	s.allegato("37", "7120220_A.pdf")
	s.componente("20", "7120200", db.TipoComponenteFinito)
	s.componente("21", "7120210", db.TipoComponenteCommerciale)
	s.comp("21").Rev = maStr("B")
	s.componente("22", "7120220", db.TipoComponenteSciolto)
	s.componente("23", "7120230", db.TipoComponenteSciolto)
	s.relazione("20", "21", 4)
	s.relazione("20", "22", 2)
	s.documento("51", "20", db.TipoDocumentoDisegno2d, "34", db.StatoNasInCoda, maStr("B"))
	s.documento("52", "21", db.TipoDocumentoDisegno2d, "36", db.StatoNasErrore, maStr("B"))
	s.documento("53", "22", db.TipoDocumentoDisegno2d, "37", db.StatoNasScritto, maStr("A"))
	s.proposta("70", "35", db.TipoDocumentoSviluppoDxf, db.StatoPropostaAperta, "20")
	s.deroga("c0", "21", db.TipoDocumentoCad3d)

	d51, d52, d53 := s.disegno("34", "51", "7120200", maStr("B")), s.disegno("36", "52", "7120210", maStr("B")),
		s.disegno("37", "53", "7120220", maStr("A"))
	v20 := s.maVoce("20", db.TipoDocumentoDisegno2d, true, valutazione.EsitoPresente)
	v20.Disegni = &valutazione.GruppoDisegni2D{Primario: &d51, RegolaPrimario: valutazione.RegolaPrimarioCorrente}
	s.maConDocumento(&v20, "51")
	v21 := s.maVoce("21", db.TipoDocumentoDisegno2d, false, valutazione.EsitoPresente)
	v21.Disegni = &valutazione.GruppoDisegni2D{Primario: &d52, RegolaPrimario: valutazione.RegolaPrimarioCorrente}
	s.maConDocumento(&v21, "52")
	v21b := s.maVoce("21", db.TipoDocumentoCad3d, false, valutazione.EsitoManca)
	v21b.DerogaID = maUp("c0")
	v20d := s.maVoce("20", db.TipoDocumentoSviluppoDxf, true, valutazione.EsitoDaVerificare)
	v20d.Motivo, v20d.PropostaAperta, v20d.FileCandidato = valutazione.MotivoFabbisognoAssociazioneNonConfermata, maUp("70"), maUp("35")
	v22 := s.maVoce("22", db.TipoDocumentoDisegno2d, false, valutazione.EsitoPresente)
	v22.Disegni = &valutazione.GruppoDisegni2D{Primario: &d53, RegolaPrimario: valutazione.RegolaPrimarioCorrente}
	s.maConDocumento(&v22, "53")

	p := maProdotto("20", "7120200")
	p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteAssente, Motivo: valutazione.MotivoFonteNessunaFonte, EsitoVista: "mancante",
		Calcolata: true}
	p.BOM = maBOMSenzaFonte(valutazione.MotivoAsseNessunaStruttura)
	n20 := s.maSenzaSTEP("20", true)
	n21 := s.maSenzaSTEP("21", false, maPadre(maRC("20"), 4, true))
	n21.Disegni = *v21.Disegni
	n22 := s.maSenzaSTEP("22", false, maPadre(maRC("20"), 2, true))
	n22.Disegni = *v22.Disegni
	n20.Disegni = *v20.Disegni
	p.ConfermatiSenzaSTEP = []valutazione.NodoBOM{n20, n21, n22}
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci: []valutazione.VoceFabbisogno{v20, v20d, v21b, v21, v22},
		Previsti: []valutazione.FabbisognoPrevisto{
			maPrevisto(maRC("23"), "7120230", nil, valutazione.MotivoPrevistoNodoDecisoFuoriPerimetro)},
		NonBloccanti: []valutazione.FabbisognoInformativo{{ComponenteID: maU("22"), TipoDocumento: string(db.TipoDocumentoSviluppoDxf),
			EsitoVista: "manca"}},
		MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
	p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true,
		FileNonTerminali: []uuid.UUID{maU("35")}, Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoAssociazioneNonConfermata}}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoNomenclatura,
		valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoSmistamento, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}

	for _, x := range []string{"34", "36", "37"} {
		s.assoc(x).Proposta, s.assoc(x).Origine = []string{p.Rif}, ancoraggio.OrigineConfermato
	}
	s.assoc("34").DocumentoID, s.assoc("34").Confermata = maUp("51"), maUp("20")
	s.assoc("36").DocumentoID, s.assoc("36").Confermata = maUp("52"), maUp("21")
	s.assoc("37").DocumentoID, s.assoc("37").Confermata = maUp("53"), maUp("22")
	s.assoc("35").Proposta, s.assoc("35").Terminale = []string{p.Rif}, false
	s.badge("35").Badge = confronto.BadgeNuovoAncoraggio

	return s.chiudi(infoCaso{Titolo: "E5: le copie sul NAS e le voci dei documenti", Contratto: "A1d §4 E5; EB7-4", Attese: []string{
		"colonne distinte; «in coda» (2D di 7120200), «errore» (2D di 7120210), «scritto» (2D di 7120220) diversi",
		"un solo «Riprova le copie»",
		"«Conferma il documento» sulla voce DXF di 7120200 con una sola «voce» e la frase DB7-2 (BOM non verificata)",
		"«Revoca la deroga» sulla voce 3D di 7120210",
		"il previsto 7120230 (nodo deciso fuori dal perimetro) in grigio",
	}}, true)
}

// e8: il calcolo non è riuscito (§2.1; RAC-05): solo ThreadID, Vista, Scrive e la fase; l'errore com'è.
func maCasoE8() casoMotoreA {
	return casoMotoreA{
		Caso: infoCaso{Titolo: "E8: il calcolo non è riuscito", Contratto: "A1d §2.1; RAC-05", Attese: []string{
			"la striscia d'errore con il testo di Errore così com'è, nessun workspace",
			"nessun titolo della pagina che dica «calcolo non riuscito» oltre al testo di Errore",
		}},
		Scrive: true,
		Pagina: paginaMotoreA{ThreadID: maU("01"), Vista: "distinta", Testa: testaMotoreA{Fase: maFase()}, Errore: erroreTempoMotoreA,
			Scrive: true},
		Foto: fotorfq.Thread{ID: maU("01"), ClienteID: maU("02"), Stato: string(db.StatoThreadAPERTA), Oggetto: maStr(maOggetto),
			Riferimento: maStr(maRiferimento), CreatoIl: maPresa},
	}
}

// e9: il thread non è valutato (§1.1): nessuna grammatica A per il cliente.
func maCasoE9() casoMotoreA {
	s := maNuovaScena()
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.et.Valutato, s.et.Motivo = false, valutazione.MotivoThreadSenzaGrammatica
	p := maProdotto("10", "7120100")
	p.Fonte = valutazione.FonteProdotto{Motivo: valutazione.MotivoFonteNonDeterminabile}
	p.BOM = maBOMSenzaFonte(valutazione.MotivoAsseNessunaStruttura)
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiNonCalcolabile,
		Motivo: valutazione.MotivoDocumentiNonDeterminabile}
	p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoDocumentazioneNonCalc}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}
	return s.chiudi(infoCaso{Titolo: "E9: thread non valutato", Contratto: "A1d §1.1", Attese: []string{
		"la striscia «Thread non valutato» con il motivo (nessuna grammatica A per il cliente)",
		"mai «tutto bene»: la fonte non calcolata non è uno stato",
	}}, true)
}

// e10: i file da smistare, con motivo e disponibilità affiancati (§1.7).
func maCasoE10() casoMotoreA {
	s := maNuovaScena()
	s.allegato("38", "Capitolato_ACME.pdf")
	s.allegato("39", "7129999.pdf")
	s.allegato("3a", "Scansione_ACME.pdf")
	s.allegato("3b", "7120100_E.pdf")
	s.allegato("3c", "image001.png")
	s.allegato("3d", "7120100_vecchio.pdf")
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.proposta("71", "38", db.TipoDocumentoCapitolato, db.StatoPropostaAperta, "")
	s.proposta("72", "39", db.TipoDocumentoDisegno2d, db.StatoPropostaAperta, "")
	s.all("3b").Sha256 = nil
	s.all("3c").Natura = string(db.NaturaAllegatoInline)

	p := maProdotto("10", "7120100")
	p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteAssente, Motivo: valutazione.MotivoFonteNessunaFonte, EsitoVista: "mancante",
		Calcolata: true}
	p.BOM = maBOMSenzaFonte(valutazione.MotivoAsseNessunaStruttura)
	p.ConfermatiSenzaSTEP = []valutazione.NodoBOM{s.maSenzaSTEP("10", true)}
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci:            []valutazione.VoceFabbisogno{s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca)},
		MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
	p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true,
		FileNonTerminali: []uuid.UUID{maU("38"), maU("39"), maU("3a"), maU("3b")},
		Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoFileDaSmistare, valutazione.MotivoSmistamentoNessunCandidato,
			valutazione.MotivoSmistamentoAssociazioneAmbigua}}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoNomenclatura,
		valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoSmistamento, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}
	s.et.DaSmistare = []valutazione.FileDaSmistare{
		{AllegatoID: maU("38"), NomeFile: "Capitolato_ACME.pdf", Motivo: valutazione.MotivoSmistamentoNessunCandidato, Prodotti: []string{p.Rif}},
		{AllegatoID: maU("39"), NomeFile: "7129999.pdf", Motivo: valutazione.MotivoSmistamentoAssociazioneAmbigua, Prodotti: []string{p.Rif}},
		{AllegatoID: maU("3a"), NomeFile: "Scansione_ACME.pdf", Motivo: valutazione.MotivoSmistamentoNessunCandidato, Orfano: true},
		{AllegatoID: maU("3b"), NomeFile: "7120100_E.pdf", Motivo: valutazione.MotivoSmistamentoNessunCandidato, Prodotti: []string{p.Rif}},
	}

	s.file("39").Disponibilita = ancoraggio.DisponibilitaPendente
	s.file("3a").Disponibilita = ancoraggio.DisponibilitaSenzaTesto
	s.file("3b").Disponibilita = ancoraggio.DisponibilitaMancante
	s.file("3c").Disegno, s.file("3c").Motivo = false, valutazione.MotivoFileNaturaNonFile
	for _, x := range []string{"38", "3a", "3b"} {
		a := s.assoc(x)
		a.Associazione, a.Collocazione, a.Terminale = ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile, false
	}
	a39 := s.assoc("39")
	a39.Associazione, a39.Collocazione, a39.Proposta, a39.Terminale = ancoraggio.AssociazioneAmbiguo, ancoraggio.CollocazioneNonDeterminabile,
		[]string{p.Rif}, false
	a3c := s.assoc("3c")
	a3c.Associazione, a3c.Collocazione, a3c.Perimetro = ancoraggio.AssociazioneNonValutata, "", valutazione.PerimetroInline
	s.assoc("3d").Esclusa = true
	s.badge("3c").Motivo = confronto.MotivoNonValutato

	return s.chiudi(infoCaso{Titolo: "E10: i file da smistare", Contratto: "A1d §1.7", Attese: []string{
		"motivo e disponibilità affiancati per ogni file da smistare",
		"la miniatura solo dove Indice.Anteprima è disponibile; «anteprima non disponibile» per 7120100_E.pdf (mancante, senza sha)",
		"una sola «voce» per conferma",
		"fuori perimetro (image001.png, inline), scartati (7120100_vecchio.pdf), orfani (Scansione_ACME.pdf) e avvisi a parte",
	}}, true)
}

// e11: e3 in sola lettura (§2.2).
func maCasoE11() casoMotoreA {
	c := maCasoE3()
	c.Scrive, c.Pagina.Scrive = false, false
	c.Caso = infoCaso{Titolo: "E11: la BOM di lavoro di E3 in sola lettura", Contratto: "A1d §2.2", Attese: []string{
		"ogni gesto spento; nessun draggable, nessun data-ma-drop",
		"«Autorizza», «Conferma l'albero» e il tipo spenti",
	}}
	return c
}

// e12: il prodotto senza componente (T-B0-07).
func maCasoE12() casoMotoreA {
	s := maNuovaScena()
	s.th.Identificativi = []fotorfq.Identificativo{{Codice: "7120300", Origine: string(db.OrigineIdentificativoManuale),
		ConfermatoDa: maUp("b0"), CreatoIl: maPresa}}
	p := valutazione.ProdottoValutato{Rif: "identificativo:7120300", Autorita: ancoraggio.AutoritaConfermata, CodiceRichiesto: "7120300",
		Base: motorea.BaseLetta{Originale: "7120300", Normalizzata: "7120300", Completa: true}, Identita: valutazione.IdentitaConfermata,
		Fonte: valutazione.FonteProdotto{Stato: valutazione.FonteAssente, Motivo: valutazione.MotivoFonteProdottoSenzaComponente,
			Calcolata: true},
		Struttura: ancoraggio.StatoStrutturaNessuna, BOM: maBOMSenzaFonte(valutazione.MotivoAsseTargetSenzaComponente),
		Documenti: valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta,
			Motivo: valutazione.MotivoDocumentiTargetSenzaComponente, MotivoPerimetro: valutazione.MotivoPerimetroTargetSenzaComponente},
		Smistamento: valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoVerificato, Calcolata: true},
		Stato:       valutazione.ProdottoNonPronto,
		Motivi: []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoNomenclatura,
			valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoDocumentazioneIncompleta}}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}
	return s.chiudi(infoCaso{Titolo: "E12: il prodotto senza componente", Contratto: "T-B0-07; D-PF-10", Attese: []string{
		"nessun gruppo: «il prodotto non ha ancora un componente»",
		"«Autorizza» spento con il motivo",
		"nessun gesto con {cid}",
	}}, true)
}

// e13: il prodotto pronto, con le copie ancora in corso (§1.5: «pronto non vuol dire partito»).
func maCasoE13() casoMotoreA {
	s := maNuovaScena()
	s.allegato("30", "7120100_C.stp")
	s.allegato("32", "7120110_A.pdf")
	s.allegato("33", "7120100_A.pdf")
	s.allegato("3c", "7120110_A1.pdf")
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.componente("11", "7120110", db.TipoComponenteSciolto)
	s.comp("11").Rev = maStr("A")
	s.documento("50", "10", db.TipoDocumentoCad3d, "30", db.StatoNasInCoda, nil)
	s.comp("10").StepStrutturaleID = maUp("50")
	s.documento("51", "11", db.TipoDocumentoDisegno2d, "32", db.StatoNasErrore, maStr("A"))
	s.documento("53", "11", db.TipoDocumentoDisegno2d, "3c", db.StatoNasScritto, maStr("A"))
	s.documento("54", "10", db.TipoDocumentoDisegno2d, "33", db.StatoNasScritto, maStr("A"))
	s.relazione("10", "11", 2)
	s.rigaComp("91", "30", "#20", db.StatoPropostaConfermata, "11")

	d51, d53, d54 := s.disegno("32", "51", "7120110", maStr("A")), s.disegno("3c", "53", "7120110", maStr("A")),
		s.disegno("33", "54", "7120100", maStr("A"))
	g11 := valutazione.GruppoDisegni2D{Primario: &d51, Alternativi: []valutazione.Disegno2D{d53},
		RegolaPrimario: valutazione.RegolaPrimarioRecente}
	g10 := valutazione.GruppoDisegni2D{Primario: &d54, RegolaPrimario: valutazione.RegolaPrimarioCorrente}
	radice := maRN("30", "#10")
	n10 := maNodoF("30", "#10", "7120100")
	maRadice(&n10)
	n10.Tipo, n10.OrigineTipo, n10.Classificazione = string(db.TipoComponenteFinito), valutazione.OrigineTipoConfermato,
		maClsTipo(db.TipoComponenteFinito, true)
	n10.Disegni = g10
	n20 := maNodoF("30", "#20", "7120110", maPadre(radice, 2, true))
	s.maDecidi(&n20, "11", false, "91")
	n20.Disegni, n20.Associazione = g11, maOrigine(ancoraggio.OrigineConfermato)

	v3d := s.maVoce("10", db.TipoDocumentoCad3d, true, valutazione.EsitoPresente)
	s.maConDocumento(&v3d, "50")
	v10 := s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoPresente)
	v10.Disegni = &g10
	s.maConDocumento(&v10, "54")
	v11 := s.maVoce("11", db.TipoDocumentoDisegno2d, false, valutazione.EsitoPresente)
	v11.Disegni = &g11
	s.maConDocumento(&v11, "51")

	p := maProdotto("10", "7120100")
	p.Fonte = s.maFonteConfermata("50", "30", "#10")
	p.Struttura = ancoraggio.StatoBOMDiLavoroProposta
	p.BOM = valutazione.VerificaBOM{Nomenclatura: maAsse(valutazione.StatoAsseVerificata, "", 0),
		Gerarchia: maAsse(valutazione.StatoAsseVerificata, "", 0), Verificata: true, FonteRegistrata: true}
	p.Nodi = []valutazione.NodoBOM{n10, n20}
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiCompleta, Voci: []valutazione.VoceFabbisogno{v3d, v10, v11},
		PerimetroChiuso: true}
	p.Verificato, p.Stato = true, valutazione.ProdottoProntoFattibilita
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}

	for x, d := range map[string]string{"30": "50", "32": "51", "33": "54", "3c": "53"} {
		a := s.assoc(x)
		a.Proposta, a.DocumentoID, a.Origine = []string{p.Rif}, maUp(d), ancoraggio.OrigineConfermato
	}
	s.assoc("30").Collocazione = ancoraggio.CollocazioneRadice
	s.assoc("30").Confermata, s.assoc("33").Confermata = maUp("10"), maUp("10")
	s.assoc("32").Confermata, s.assoc("3c").Confermata = maUp("11"), maUp("11")

	return s.chiudi(infoCaso{Titolo: "E13: prodotto pronto, copie sul NAS ancora in corso",
		Contratto: "A1d §1.5", Attese: []string{
			"lo stato del prodotto «pronto» e le copie aperte (errore, in coda) visibili insieme",
			"nessuna frase DB7-2 (la BOM è verificata)",
			"«Sostituisci il principale con questo» sull'alternativo 7120110_A1.pdf",
			"la fase registrata non cambia",
		}}, true)
}

// e14: un nodo spostato rispetto alle relazioni confermate (ELENCO §4.5; D5 di P7e).
func maCasoE14() casoMotoreA {
	s := maNuovaScena()
	s.allegato("30", "7120100_C.stp")
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.componente("11", "7120110", db.TipoComponenteSottoassieme)
	s.componente("12", "7120120", db.TipoComponenteSciolto)
	s.documento("50", "10", db.TipoDocumentoCad3d, "30", db.StatoNasScritto, nil)
	s.comp("10").StepStrutturaleID = maUp("50")
	s.relazione("10", "11", 1)
	s.relazione("10", "12", 3)
	s.rigaComp("91", "30", "#20", db.StatoPropostaConfermata, "11")
	s.rigaComp("92", "30", "#30", db.StatoPropostaConfermata, "12")

	radice := maRN("30", "#10")
	n10 := maNodoF("30", "#10", "7120100")
	maRadice(&n10)
	n10.Tipo, n10.OrigineTipo, n10.Classificazione = string(db.TipoComponenteFinito), valutazione.OrigineTipoConfermato,
		maClsTipo(db.TipoComponenteFinito, true)
	n20 := maNodoF("30", "#20", "7120110", maPadre(radice, 1, true))
	s.maDecidi(&n20, "11", false, "91")
	n30 := maNodoF("30", "#30", "7120120", maPadre(maRN("30", "#20"), 2, false))
	s.maDecidi(&n30, "12", false, "92")

	p := maProdotto("10", "7120100")
	p.Fonte = s.maFonteConfermata("50", "30", "#10")
	p.Struttura = ancoraggio.StatoBOMDiLavoroProposta
	p.BOM = valutazione.VerificaBOM{Nomenclatura: maAsse(valutazione.StatoAsseVerificata, "", 0),
		Gerarchia: maAsse(valutazione.StatoAsseDaVerificare, valutazione.MotivoAsseDaDecidere, 1), FonteRegistrata: true}
	p.Nodi = []valutazione.NodoBOM{n10, n20, n30}
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci: []valutazione.VoceFabbisogno{
			s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca),
			s.maVoce("11", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca),
			s.maVoce("12", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca)},
		MotivoPerimetro: valutazione.MotivoPerimetroArchiDaDecidere}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}
	a := s.assoc("30")
	a.Collocazione, a.Proposta, a.DocumentoID, a.Confermata, a.Origine = ancoraggio.CollocazioneRadice, []string{p.Rif}, maUp("50"),
		maUp("10"), ancoraggio.OrigineConfermato

	return s.chiudi(infoCaso{Titolo: "E14: un nodo nel file sotto un padre, confermato sotto un altro",
		Contratto: "ELENCO §4.5; D5 di P7e", Attese: []string{
			"#30 (7120120) sotto #20 con il tratteggio; il badge «confermato sotto 7120100 ✓×3»",
			"Posizione: «Nel file: sotto 7120110 ×2 · da confermare» e «Relazioni confermate correnti: sotto 7120100 ✓×3 · solo confermata»",
			"«Da» di Sposta: la sola opzione 00000000-0000-4000-8000-000000000010",
			"«Scollega da 7120100»; data-ma-da = 00000000-0000-4000-8000-000000000010",
		}}, true)
}

// maBOMDecisa: la scena comune di e15, e16 ed e19: lo STEP 30 confermato (documento 50) per 7120100, la radice #10 e i
// nodi decisi sotto di lei (chiave, componente, riga), ognuno con la relazione confermata ×1.
func maBOMDecisa(s *maScena, figli ...[3]string) (valutazione.ProdottoValutato, []valutazione.NodoBOM) {
	s.allegato("30", "7120100_C.stp")
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.documento("50", "10", db.TipoDocumentoCad3d, "30", db.StatoNasScritto, nil)
	s.comp("10").StepStrutturaleID = maUp("50")
	radice := maRN("30", "#10")
	n10 := maNodoF("30", "#10", "7120100")
	maRadice(&n10)
	n10.Tipo, n10.OrigineTipo, n10.Classificazione = string(db.TipoComponenteFinito), valutazione.OrigineTipoConfermato,
		maClsTipo(db.TipoComponenteFinito, true)
	nodi := []valutazione.NodoBOM{n10}
	for _, f := range figli {
		chiave, comp, riga := f[0], f[1], f[2]
		s.relazione("10", comp, 1)
		s.rigaComp(riga, "30", chiave, db.StatoPropostaConfermata, comp)
		n := maNodoF("30", chiave, s.comp(comp).Codice, maPadre(radice, 1, true))
		s.maDecidi(&n, comp, false, riga)
		nodi = append(nodi, n)
	}
	p := maProdotto("10", "7120100")
	p.Fonte = s.maFonteConfermata("50", "30", "#10")
	p.Struttura = ancoraggio.StatoBOMDiLavoroProposta
	p.BOM = valutazione.VerificaBOM{Nomenclatura: maAsse(valutazione.StatoAsseVerificata, "", 0),
		Gerarchia: maAsse(valutazione.StatoAsseVerificata, "", 0), Verificata: true, FonteRegistrata: true}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoDocumentazioneIncompleta}
	a := s.assoc("30")
	a.Collocazione, a.Proposta, a.DocumentoID, a.Confermata, a.Origine = ancoraggio.CollocazioneRadice, []string{p.Rif}, maUp("50"),
		maUp("10"), ancoraggio.OrigineConfermato
	return p, nodi
}

// e15: i documenti correnti dello stesso tipo per la scelta «aggiungi» o «sostituisci» (ELENCO §4.11).
func maCasoE15() casoMotoreA {
	s := maNuovaScena()
	s.componente("11", "7120110", db.TipoComponenteSciolto)
	s.componente("12", "7120120", db.TipoComponenteSciolto)
	p, nodi := maBOMDecisa(s, [3]string{"#20", "11", "91"}, [3]string{"#30", "12", "92"})
	s.allegato("31", "7120110_A.pdf")
	s.allegato("32", "7120120_A.pdf")
	s.allegato("33", "7120120_B.pdf")
	s.documento("54", "11", db.TipoDocumentoDisegno2d, "31", db.StatoNasScritto, maStr("A"))
	s.documento("55", "12", db.TipoDocumentoDisegno2d, "32", db.StatoNasScritto, maStr("A"))
	s.proposta("70", "33", db.TipoDocumentoDisegno2d, db.StatoPropostaAperta, "12")

	d54, d55, d33 := s.disegno("31", "54", "7120110", maStr("A")), s.disegno("32", "55", "7120120", maStr("A")),
		s.disegno("33", "", "7120120", maStr("B"))
	v11 := s.maVoce("11", db.TipoDocumentoDisegno2d, false, valutazione.EsitoPresente)
	v11.Disegni = &valutazione.GruppoDisegni2D{Primario: &d54, RegolaPrimario: valutazione.RegolaPrimarioCorrente}
	s.maConDocumento(&v11, "54")
	v12 := s.maVoce("12", db.TipoDocumentoDisegno2d, false, valutazione.EsitoDaVerificare)
	v12.Disegni = &valutazione.GruppoDisegni2D{Primario: &d55, Alternativi: []valutazione.Disegno2D{d33},
		RegolaPrimario: valutazione.RegolaPrimarioCorrente}
	s.maConDocumento(&v12, "55")
	v12.Motivo, v12.PropostaAperta, v12.FileCandidato = valutazione.MotivoFabbisognoAssociazioneNonConfermata, maUp("70"), maUp("33")
	nodi[1].Disegni, nodi[2].Disegni = *v11.Disegni, *v12.Disegni
	p.Nodi = nodi
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci:            []valutazione.VoceFabbisogno{s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca), v11, v12},
		PerimetroChiuso: true}
	p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true,
		FileNonTerminali: []uuid.UUID{maU("33")}, Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoAssociazioneNonConfermata}}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}
	for x, d := range map[string][2]string{"31": {"54", "11"}, "32": {"55", "12"}} {
		a := s.assoc(x)
		a.Proposta, a.DocumentoID, a.Confermata, a.Origine = []string{p.Rif}, maUp(d[0]), maUp(d[1]), ancoraggio.OrigineConfermato
	}
	s.assoc("33").Proposta, s.assoc("33").Terminale = []string{p.Rif}, false
	s.badge("33").Badge = confronto.BadgeNuovoAncoraggio

	return s.chiudi(infoCaso{Titolo: "E15: i 2D correnti dello stesso tipo, per «aggiungi» o «sostituisci»",
		Contratto: "ELENCO §4.11", Attese: []string{
			"nel cassetto select[data-ma-scelte] senza preselezione",
			"per 7120120 un fieldset con «aggiungi» e «sostituisci:00000000-0000-4000-8000-000000000055», non spuntati",
			"nessuna «scelta» nel form con «proposta»",
		}}, true)
}

// e16: rev e descrizione registrate, e il componente archiviato (ELENCO §4.6).
func maCasoE16() casoMotoreA {
	s := maNuovaScena()
	s.componente("11", "7120110", db.TipoComponenteSciolto)
	s.comp("11").Rev, s.comp("11").Descrizione = maStr("B"), maStr("Staffa di prova ACME")
	s.componente("12", "7120120", db.TipoComponenteSciolto)
	s.componente("15", "7120150", db.TipoComponenteSciolto)
	archiviato := maPresa.Add(-24 * time.Hour)
	s.comp("15").ArchiviatoIl = &archiviato
	p, nodi := maBOMDecisa(s, [3]string{"#20", "11", "91"}, [3]string{"#30", "12", "92"})
	s.rigaComp("95", "30", "#50", db.StatoPropostaConfermata, "15")
	n50 := maNodoF("30", "#50", "7120150", maPadre(maRN("30", "#10"), 1, false))
	// il componente archiviato non è fra i confermati del contesto (valutazione/ancoraggi.go contestoStrutturale): il
	// nodo non ha la Decisione né il codice confermato, resta la riga decisa con il suo componente (integrazione di
	// A1d-Q3: prima il nodo aveva la Decisione su 15, che il motore non dà, e la pagina offriva «Archivia»)
	s.maProponi(&n50, db.TipoComponenteSciolto, valutazione.OrigineTipoPropostoDallaRegola, "")
	r95 := s.riga("95")
	n50.Nodo.RigaDecisa = &ancoraggio.RigaDecisaLegacy{ID: r95.ID, AllegatoID: r95.AllegatoID, Sha256: r95.Sha256,
		Chiave: r95.Chiave, Stato: r95.Stato, ComponenteID: r95.ComponenteID, DecisoDa: r95.DecisoDa}
	n50.Nodo.DecisoDaPersona = true
	nodi[1].Descrizione = maStr("Staffa di prova ACME")
	nodi[2].Descrizione = maStr("Piastra ACME dallo STEP")
	p.Nodi = append(nodi, n50)
	p.BOM.Gerarchia, p.BOM.Verificata = maAsse(valutazione.StatoAsseDaVerificare, valutazione.MotivoAsseDaDecidere, 1), false
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci: []valutazione.VoceFabbisogno{
			s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca),
			s.maVoce("11", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca),
			s.maVoce("12", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca)},
		MotivoPerimetro: valutazione.MotivoPerimetroArchiDaDecidere}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}

	return s.chiudi(infoCaso{Titolo: "E16: rev e descrizione registrate; un componente archiviato",
		Contratto: "ELENCO §4.6", Attese: []string{
			"«Salva rev e descrizione» acceso e precompilato (B, «Staffa di prova ACME») per 7120110",
			"spento con il motivo per 7120120 (nessuna descrizione registrata: quella del nodo viene dallo STEP)",
			"«Ripristina» per 7120150 (archiviato); nessun «Archivia» per 7120150",
		}}, true)
}

// e17: i file da smistare con i prodotti del file e quelli del messaggio (R106 B, R107, F0-18).
func maCasoE17() casoMotoreA {
	s := maNuovaScena()
	s.componente("11", "7120110", db.TipoComponenteSciolto)
	p1, nodi := maBOMDecisa(s, [3]string{"#20", "11", "91"})
	p1.Nodi = nodi
	s.componente("20", "7120200", db.TipoComponenteFinito)
	s.componente("21", "7120210", db.TipoComponenteSciolto)
	s.relazione("20", "21", 4)
	s.allegato("3a", "7120110_C.pdf")
	s.allegato("3b", "7120999.pdf")
	s.allegato("3c", "Disegno_ACME.pdf")
	// una proposta aperta per file: «Associa a…» di R8 sta solo dentro Indice.PropostaAperta (contratto §3.4 C), e il
	// piano del frontend §8.2 chiede i suoi optgroup (integrazione di A1d-Q3: prima il caso non aveva proposte)
	s.proposta("70", "3a", db.TipoDocumentoDisegno2d, db.StatoPropostaAperta, "11")
	s.proposta("71", "3b", db.TipoDocumentoDisegno2d, db.StatoPropostaAperta, "")
	s.proposta("72", "3c", db.TipoDocumentoDisegno2d, db.StatoPropostaAperta, "")

	p1.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci: []valutazione.VoceFabbisogno{
			s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca),
			s.maVoce("11", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca)},
		PerimetroChiuso: true}
	p1.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true,
		FileNonTerminali: []uuid.UUID{maU("3a"), maU("3b")}, Motivi: []valutazione.MotivoSmistamento{
			valutazione.MotivoSmistamentoFileDaSmistare, valutazione.MotivoSmistamentoAssociazioneDiscordante}}
	p1.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoSmistamento, valutazione.MotivoProdottoDocumentazioneIncompleta}
	p2 := maProdotto("20", "7120200")
	p2.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteAssente, Motivo: valutazione.MotivoFonteNessunaFonte, EsitoVista: "mancante",
		Calcolata: true}
	p2.BOM = maBOMSenzaFonte(valutazione.MotivoAsseNessunaStruttura)
	p2.ConfermatiSenzaSTEP = []valutazione.NodoBOM{s.maSenzaSTEP("20", true), s.maSenzaSTEP("21", false, maPadre(maRC("20"), 4, true))}
	p2.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci: []valutazione.VoceFabbisogno{
			s.maVoce("20", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca),
			s.maVoce("21", db.TipoDocumentoDisegno2d, false, valutazione.EsitoManca)},
		MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
	p2.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true,
		FileNonTerminali: []uuid.UUID{maU("3b")}, Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoFileDaSmistare}}
	p2.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoNomenclatura,
		valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoSmistamento, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p1, p2}
	s.et.DaSmistare = []valutazione.FileDaSmistare{
		{AllegatoID: maU("3a"), NomeFile: "7120110_C.pdf", Motivo: valutazione.MotivoSmistamentoAssociazioneDiscordante,
			Prodotti: []string{p1.Rif}},
		{AllegatoID: maU("3b"), NomeFile: "7120999.pdf", Motivo: valutazione.MotivoSmistamentoNessunCandidato, Orfano: true,
			ProdottiContesto: []string{p1.Rif, p2.Rif}},
		{AllegatoID: maU("3c"), NomeFile: "Disegno_ACME.pdf", Motivo: valutazione.MotivoSmistamentoNessunCandidato, Orfano: true},
	}
	a3a := s.assoc("3a")
	a3a.Associazione, a3a.Proposta, a3a.Pertinente, a3a.Terminale = ancoraggio.AssociazioneDiscordante, []string{p1.Rif},
		[]string{p1.Rif}, false
	a3b := s.assoc("3b")
	a3b.Associazione, a3b.Collocazione, a3b.Terminale = ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile, false
	a3b.PertinenzaContesto = []string{p1.Rif}
	a3b.Contesto = &valutazione.ContestoMessaggio{MessaggioID: maU("03"), Letture: []string{"7120100", "7120200"},
		Prodotti: []string{p1.Rif, p2.Rif}}
	a3c := s.assoc("3c")
	a3c.Associazione, a3c.Collocazione, a3c.Terminale = ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile, false
	s.badge("3a").Badge, s.badge("3a").Motivo = confronto.BadgeDiverso, confronto.MotivoBaseDiversa

	return s.chiudi(infoCaso{Titolo: "E17: file da smistare, prodotti del file e prodotti del messaggio",
		Contratto: "R106 B; R107; F0-18", Attese: []string{
			"7120110_C.pdf: optgroup «del file · 7120100»",
			"7120999.pdf: optgroup «dal messaggio · 7120100» e «dal messaggio · 7120200»",
			"Disegno_ACME.pdf: «prodotto scelto (il file non ha prodotti)» con l'avviso",
			"nessuna opzione selezionata; contesto, pertinenza e pertinenza dal contesto distinti",
		}}, true)
}

// e18: lo stesso Rif in due strutture candidate dello stesso file (D6 di P7e).
func maCasoE18() casoMotoreA {
	s := maNuovaScena()
	s.allegato("30", "7120100_C.stp")
	s.componente("10", "7120100", db.TipoComponenteFinito)
	s.documento("50", "10", db.TipoDocumentoCad3d, "30", db.StatoNasScritto, nil)
	s.rigaComp("90", "30", "#10", db.StatoPropostaAperta, "")
	s.rigaComp("91", "30", "#20", db.StatoPropostaAperta, "")
	s.rigaComp("92", "30", "#30", db.StatoPropostaAperta, "")

	nodo := func(chiave, codice, riga string, padri ...valutazione.Parentela) valutazione.NodoBOM {
		n := maNodoF("30", chiave, codice, padri...)
		s.maProponi(&n, db.TipoComponenteSottoassieme, valutazione.OrigineTipoPropostoDallaRegola, riga)
		return n
	}
	a10 := nodo("#10", "7120100", "90")
	maRadice(&a10)
	a20 := nodo("#20", "7120100-1", "91", maPadre(maRN("30", "#10"), 1, false))
	a30 := nodo("#30", "7120130", "92", maPadre(maRN("30", "#20"), 2, false))
	a30.Tipo = string(db.TipoComponenteSciolto)
	b20 := nodo("#20", "7120100-1", "91")
	b30 := nodo("#30", "7120130", "92", maPadre(maRN("30", "#20"), 2, false))
	b30.Tipo = string(db.TipoComponenteSciolto)

	p := maProdotto("10", "7120100")
	p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteDaScegliere,
		EsitoVista: "da_scegliere", Calcolata: true, Candidati: []ancoraggio.DocumentoCandidato{
			maCandidato(ancoraggio.CandidatoDaDocumentoDelProdotto, "30", "50", []string{"#10", "#20"}, motorea.CompatibilitaUguale,
				ancoraggio.EstrazioneRiuscita, true)}}
	p.Struttura = ancoraggio.StatoStrutturaCandidata
	p.BOM = maBOMSenzaFonte(valutazione.MotivoAsseFonteNonConfermata)
	p.StruttureCandidate = []valutazione.StrutturaCandidata{
		maCandidata("30", "#10", true, motorea.CompatibilitaUguale, true, a10, a20, a30),
		maCandidata("30", "#20", false, motorea.CompatibilitaParziale, true, b20, b30)}
	p.ConfermatiSenzaSTEP = []valutazione.NodoBOM{s.maSenzaSTEP("10", true)}
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante,
		Voci:            []valutazione.VoceFabbisogno{s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca)},
		MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
	p.Motivi = []valutazione.MotivoProdotto{valutazione.MotivoProdottoFonte, valutazione.MotivoProdottoNomenclatura,
		valutazione.MotivoProdottoGerarchia, valutazione.MotivoProdottoDocumentazioneIncompleta}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}
	a := s.assoc("30")
	a.Collocazione, a.Proposta, a.DocumentoID, a.Confermata, a.Origine = ancoraggio.CollocazioneRadice, []string{p.Rif}, maUp("50"),
		maUp("10"), ancoraggio.OrigineConfermato

	return s.chiudi(infoCaso{Titolo: "E18: lo stesso Rif in due strutture candidate dello stesso file",
		Contratto: "D6 di P7e", Attese: []string{
			"le chiavi «cand:…:#10|…#30» e «cand:…:#20|…#30» distinte",
			"scegliere #30 nella candidata A non mostra il pannello della candidata B",
			"«Autorizza» con AutorizzazioneDiStruttura diversa per (30, #10) (documento 50) e (30, #20) (delega del nodo)",
		}}, true)
}

// e19: la voce da decidere con un 2D corrente già registrato (U-08, R-FE-16).
func maCasoE19() casoMotoreA {
	s := maNuovaScena()
	s.componente("11", "7120110", db.TipoComponenteSciolto)
	p, nodi := maBOMDecisa(s, [3]string{"#20", "11", "91"})
	s.allegato("31", "7120110_A.pdf")
	s.allegato("40", "7120110_B.pdf")
	s.documento("56", "11", db.TipoDocumentoDisegno2d, "31", db.StatoNasScritto, maStr("A"))
	s.proposta("73", "40", db.TipoDocumentoDisegno2d, db.StatoPropostaAperta, "11")

	d56, d40 := s.disegno("31", "56", "7120110", maStr("A")), s.disegno("40", "", "7120110", maStr("B"))
	v11 := s.maVoce("11", db.TipoDocumentoDisegno2d, false, valutazione.EsitoDaVerificare)
	v11.Disegni = &valutazione.GruppoDisegni2D{Primario: &d56, Alternativi: []valutazione.Disegno2D{d40},
		RegolaPrimario: valutazione.RegolaPrimarioCorrente}
	s.maConDocumento(&v11, "56")
	v11.Motivo, v11.PropostaAperta, v11.FileCandidato = valutazione.MotivoFabbisognoAssociazioneNonConfermata, maUp("73"), maUp("40")
	nodi[1].Disegni = *v11.Disegni
	p.Nodi = nodi
	p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceNonPresente,
		Voci:            []valutazione.VoceFabbisogno{s.maVoce("10", db.TipoDocumentoDisegno2d, true, valutazione.EsitoManca), v11},
		PerimetroChiuso: true}
	p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true,
		FileNonTerminali: []uuid.UUID{maU("40")}, Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoAssociazioneNonConfermata}}
	s.et.ProdottiValutati = []valutazione.ProdottoValutato{p}
	a := s.assoc("31")
	a.Proposta, a.DocumentoID, a.Confermata, a.Origine = []string{p.Rif}, maUp("56"), maUp("11"), ancoraggio.OrigineConfermato
	s.assoc("40").Proposta, s.assoc("40").Terminale = []string{p.Rif}, false
	s.badge("40").Badge = confronto.BadgeNuovoAncoraggio

	return s.chiudi(infoCaso{Titolo: "E19: la voce da decidere, con un 2D corrente già registrato",
		Contratto: "U-08; R-FE-16; D-PF-16", Attese: []string{
			"«Conferma il documento» con una sola «voce» (il server la rifiuterà: «no»)",
			"nel cassetto i correnti dello stesso tipo (7120110_A.pdf) e la nota: «Se la conferma chiede di scegliere fra aggiungere e sostituire, in questa pagina si rifiuta (U-08): la scelta si fa nel Fascicolo.»",
		}}, true)
}

// ---- le prove ----

// TestMotoreACasiCoincidonoConIlCostruttore (QFE-01): i diciassette casi, nell'ordine e con i nomi di §8.2; con
// MA_CASI_AGGIORNA=1 si scrivono, senza si confrontano byte per byte con testdata/motore_a. I file e*_*.json su disco
// sono esattamente i casi del costruttore (RAC-04: nessun altro, nessuno in meno).
func TestMotoreACasiCoincidonoConIlCostruttore(t *testing.T) {
	casi := maCasiACME()
	if len(casi) != len(maNomiCasi) {
		t.Fatalf("%d casi, attesi %d", len(casi), len(maNomiCasi))
	}
	aggiorna := os.Getenv("MA_CASI_AGGIORNA") == "1"
	if aggiorna {
		if err := os.MkdirAll(maCartellaCasi, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range casi {
		if c.Nome != maNomiCasi[i] {
			t.Errorf("caso %d: %q, atteso %q", i, c.Nome, maNomiCasi[i])
		}
		b, err := maCodificaCaso(c.Caso)
		if err != nil {
			t.Fatalf("%s: %v", c.Nome, err)
		}
		percorso := filepath.Join(maCartellaCasi, c.Nome+".json")
		if aggiorna {
			if err := os.WriteFile(percorso, b, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		su, err := os.ReadFile(percorso)
		if err != nil {
			t.Errorf("%s: %v (rigenerare con MA_CASI_AGGIORNA=1)", c.Nome, err)
			continue
		}
		if !bytes.Equal(b, su) {
			t.Errorf("%s: il JSON su disco non è quello del costruttore (%s; rigenerare con MA_CASI_AGGIORNA=1)", c.Nome,
				maPrimaDifferenza(su, b))
		}
	}
	trovati, err := filepath.Glob(filepath.Join(maCartellaCasi, maSchemaCasi))
	if err != nil {
		t.Fatal(err)
	}
	var nomi []string
	for _, f := range trovati {
		nomi = append(nomi, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	attesi := append([]string(nil), maNomiCasi...)
	sort.Strings(nomi)
	sort.Strings(attesi)
	if strings.Join(nomi, ",") != strings.Join(attesi, ",") {
		t.Errorf("i file %s di %s sono %v, attesi %v", maSchemaCasi, maCartellaCasi, nomi, attesi)
	}
}

// maPrimaDifferenza: la prima riga diversa fra il JSON su disco e quello del costruttore.
func maPrimaDifferenza(su, nuovo []byte) string {
	a, b := strings.Split(string(su), "\n"), strings.Split(string(nuovo), "\n")
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y string
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y || i >= len(a) || i >= len(b) {
			return fmt.Sprintf("riga %d: su disco %q, dal costruttore %q", i+1, x, y)
		}
	}
	return "nessuna riga diversa"
}

// TestMotoreACasiLetturaStretta (QFE-01): ogni e*_*.json si legge con leggiCasoMotoreA (DisallowUnknownFields) e,
// riscritto, dà gli stessi byte (nessun campo perso o in più); scrive torna nella pagina; la pagina del JSON non ha né
// «scrive» né «indice» (json:"-"); un campo sconosciuto messo a qualunque profondità (dalla radice alla foglia più
// profonda) fa fallire la lettura.
func TestMotoreACasiLetturaStretta(t *testing.T) {
	trovati, err := filepath.Glob(filepath.Join(maCartellaCasi, maSchemaCasi))
	if err != nil || len(trovati) == 0 {
		t.Fatalf("nessun caso in %s (%v)", maCartellaCasi, err)
	}
	for _, f := range trovati {
		nome := strings.TrimSuffix(filepath.Base(f), ".json")
		c := leggiCasoMotoreA(t, nome)
		if c.Pagina.Scrive != c.Scrive {
			t.Errorf("%s: Pagina.Scrive = %v, scrive = %v", nome, c.Pagina.Scrive, c.Scrive)
		}
		su, _ := os.ReadFile(f)
		b, err := maCodificaCaso(*c)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if !bytes.Equal(b, su) {
			t.Errorf("%s: riletto e riscritto non dà gli stessi byte (%s)", nome, maPrimaDifferenza(su, b))
		}
		var grezzo map[string]any
		d := json.NewDecoder(bytes.NewReader(su))
		d.UseNumber()
		if err := d.Decode(&grezzo); err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if pg, ok := grezzo["pagina"].(map[string]any); !ok {
			t.Errorf("%s: niente pagina", nome)
		} else {
			for _, k := range []string{"scrive", "indice", "Scrive", "Indice"} {
				if _, ha := pg[k]; ha {
					t.Errorf("%s: la pagina del JSON ha %q (json:\"-\")", nome, k)
				}
			}
		}
		for _, cammino := range maCamminiDiOggetti(grezzo) {
			var copia map[string]any
			d := json.NewDecoder(bytes.NewReader(su))
			d.UseNumber()
			if err := d.Decode(&copia); err != nil {
				t.Fatal(err)
			}
			maOggettoIn(copia, cammino)["campo_ignoto_acme"] = true
			b, err := json.Marshal(copia)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := maLeggiCasoStretto(b); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Errorf("%s: un campo sconosciuto in %s non fa fallire la lettura (err = %v)", nome, strings.Join(cammino, "."), err)
			}
		}
	}
}

// maCamminiDiOggetti: i cammini di un oggetto JSON per ogni profondità (il primo che si incontra a ogni livello, in
// ordine di chiave), dalla radice al più profondo. Le mappe Go del caso (confronto.conteggi) restano fuori: accettano
// qualunque chiave.
func maCamminiDiOggetti(radice map[string]any) [][]string {
	primi := map[int][]string{}
	var visita func(v any, cammino []string)
	visita = func(v any, cammino []string) {
		switch x := v.(type) {
		case map[string]any:
			if len(cammino) > 0 && cammino[len(cammino)-1] == "conteggi" {
				return
			}
			if _, c := primi[len(cammino)]; !c {
				primi[len(cammino)] = append([]string(nil), cammino...)
			}
			chiavi := make([]string, 0, len(x))
			for k := range x {
				chiavi = append(chiavi, k)
			}
			sort.Strings(chiavi)
			for _, k := range chiavi {
				visita(x[k], append(cammino, k))
			}
		case []any:
			for i, e := range x {
				visita(e, append(cammino, fmt.Sprint(i)))
			}
		}
	}
	visita(radice, nil)
	var out [][]string
	for p := 0; ; p++ {
		c, ok := primi[p]
		if !ok {
			if p > 64 {
				break
			}
			continue
		}
		out = append(out, c)
	}
	return out
}

// maOggettoIn: l'oggetto al cammino dato (le cifre sono indici di un elenco).
func maOggettoIn(radice map[string]any, cammino []string) map[string]any {
	var v any = radice
	for _, k := range cammino {
		switch x := v.(type) {
		case map[string]any:
			v = x[k]
		case []any:
			var i int
			fmt.Sscan(k, &i)
			v = x[i]
		}
	}
	return v.(map[string]any)
}

// TestMotoreACasiCoerenti (QFE-01, RAC-05, RAC-08, RAC-27): i casi del costruttore sono coerenti con sé stessi e con la
// regola dei privati: ogni riferimento (allegato, componente, documento, proposta, riga, relazione, deroga, struttura)
// trova il suo record; le strutture di ancoraggio e le righe della foto che l'indice legge ci sono; e8 ha la forma di
// RAC-05; gli UUID, gli sha e i codici sono ACME.
func TestMotoreACasiCoerenti(t *testing.T) {
	for _, c := range maCasiACME() {
		c := c
		t.Run(c.Nome, func(t *testing.T) {
			for _, e := range maControllaCaso(&c.Caso) {
				t.Error(e)
			}
			b, err := maCodificaCaso(c.Caso)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range maControllaPrivati(b) {
				t.Error(e)
			}
		})
	}
}

var (
	maReUUID   = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	maReUUIDOk = regexp.MustCompile(`^00000000-0000-4000-8000-0000000000[0-9a-f]{2}$`)
	maReSha    = regexp.MustCompile(`[0-9a-fA-F]{64}`)
	maReShaOk  = regexp.MustCompile(`^0{62}[0-9a-f]{2}$`)
	maReCifre  = regexp.MustCompile(`[0-9]+`)
	maReNegati = regexp.MustCompile(`ACME-T?[0-9]{3}(?:[^0-9A-Za-z]|$)|ISO4762|cockpit\.test|@`)
)

// maControllaPrivati: nel JSON solo UUID ACME (o lo zero dei campi vuoti), sha inventati, codici di sette cifre della
// famiglia 712xxxx, nessun codice del contratto (ACME-100, ACME-T999, ISO4762-…) né indirizzi (RAC-27).
func maControllaPrivati(b []byte) []string {
	var out []string
	testo := string(b)
	for _, u := range maReUUID.FindAllString(testo, -1) {
		if !maReUUIDOk.MatchString(u) && u != uuid.Nil.String() {
			out = append(out, "UUID non ACME: "+u)
		}
	}
	for _, s := range maReSha.FindAllString(testo, -1) {
		if !maReShaOk.MatchString(s) {
			out = append(out, "sha non inventato: "+s)
		}
	}
	for _, n := range maReCifre.FindAllString(testo, -1) {
		if len(n) == 7 && !strings.HasPrefix(n, "712") {
			out = append(out, "codice fuori dalla famiglia 712xxxx: "+n)
		}
	}
	for _, n := range maReNegati.FindAllString(testo, -1) {
		out = append(out, "testo vietato: "+n)
	}
	return out
}

// maControllaCaso: i problemi di coerenza di un caso (vuoto = coerente).
func maControllaCaso(c *casoMotoreA) []string {
	var out []string
	e := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	th, pg := &c.Foto, &c.Pagina

	if c.Caso.Titolo == "" || c.Caso.Contratto == "" || len(c.Caso.Attese) == 0 {
		e("_caso senza titolo, contratto o attese")
	}
	if pg.ThreadID != maU("01") || th.ID != pg.ThreadID || th.ClienteID != maU("02") {
		e("thread %s, foto %s, cliente %s", pg.ThreadID, th.ID, th.ClienteID)
	}
	if pg.Vista != "distinta" || pg.Scrive != c.Scrive || pg.Testa.Fase == nil {
		e("vista %q, scrive %v/%v, fase %v", pg.Vista, pg.Scrive, c.Scrive, pg.Testa.Fase)
	}
	if pg.Esito == nil {
		// RAC-05: con Errore solo ThreadID, Vista, Scrive e al più Testa.Fase.
		if pg.Errore == "" || pg.Confronto != nil || pg.Prodotto != "" || !pg.Calcolato.IsZero() || pg.Testa.Cliente != "" ||
			pg.Testa.Oggetto != "" || pg.Testa.RiferimentoCliente != "" || pg.Testa.StatoThread != "" {
			e("pagina senza esito fuori dalla forma di RAC-05: %+v", *pg)
		}
		return out
	}
	et, cf := pg.Esito, pg.Confronto
	if pg.Errore != "" || cf == nil || pg.Calcolato.IsZero() {
		e("pagina con esito: errore %q, confronto %v, calcolato %v", pg.Errore, cf != nil, pg.Calcolato)
		return out
	}
	if et.ThreadID != th.ID || et.ClienteID != th.ClienteID {
		e("esito di %s/%s", et.ThreadID, et.ClienteID)
	}
	if pg.Testa.Cliente != maCliente || th.Oggetto == nil || pg.Testa.Oggetto != *th.Oggetto || th.Riferimento == nil ||
		pg.Testa.RiferimentoCliente != *th.Riferimento || pg.Testa.StatoThread != th.Stato || et.Fascicolo.FaseThread != th.Stato {
		e("testa diversa dalla foto: %+v", pg.Testa)
	}
	if len(et.ProdottiValutati) == 0 && pg.Prodotto != "" || len(et.ProdottiValutati) > 0 && pg.Prodotto != et.ProdottiValutati[0].Rif {
		e("prodotto scelto %q", pg.Prodotto)
	}

	allegati := map[uuid.UUID]*fotorfq.Allegato{}
	for i := range th.Allegati {
		allegati[th.Allegati[i].ID] = &th.Allegati[i]
	}
	comps := map[uuid.UUID]*fotorfq.Componente{}
	for i := range th.Componenti {
		comps[th.Componenti[i].ID] = &th.Componenti[i]
	}
	docs := map[uuid.UUID]*fotorfq.DocumentoConfermato{}
	for i := range th.Documenti {
		docs[th.Documenti[i].ID] = &th.Documenti[i]
	}
	proposte := map[uuid.UUID]*fotorfq.PropostaAttuale{}
	for i := range th.Proposte {
		proposte[th.Proposte[i].ID] = &th.Proposte[i]
	}
	righe := map[uuid.UUID]*fotorfq.RigaComponenteProposta{}
	for i := range th.RigheComponenteProposta {
		righe[th.RigheComponenteProposta[i].ID] = &th.RigheComponenteProposta[i]
	}
	relazione := func(p, f uuid.UUID) *fotorfq.Relazione {
		return maTrova(th.Relazioni, func(r *fotorfq.Relazione) bool { return r.PadreID == p && r.FiglioID == f })
	}
	prodotti := map[string]*valutazione.ProdottoValutato{}
	for i := range et.ProdottiValutati {
		prodotti[et.ProdottiValutati[i].Rif] = &et.ProdottiValutati[i]
	}
	shaDi := func(a uuid.UUID) string {
		if x := allegati[a]; x != nil && x.Sha256 != nil {
			return *x.Sha256
		}
		return ""
	}
	comp := func(id uuid.UUID, dove string) {
		if comps[id] == nil {
			e("%s: componente %s assente dalla foto", dove, id)
		}
	}
	prodottiNoti := func(rif []string, dove string) {
		for _, r := range rif {
			if prodotti[r] == nil {
				e("%s: prodotto %q non valutato", dove, r)
			}
		}
	}

	// RAC-08: un record dell'esito e del confronto per ogni allegato, e nessuno di un allegato che non c'è.
	nFile, nAssoc, nBadge := map[uuid.UUID]int{}, map[uuid.UUID]int{}, map[uuid.UUID]int{}
	for i, f := range et.File {
		nFile[f.AllegatoID]++
		if i > 0 && et.File[i-1].AllegatoID.String() >= f.AllegatoID.String() {
			e("esito.file non in ordine di allegato")
		}
	}
	for i, a := range et.Associazioni {
		nAssoc[a.AllegatoID]++
		if i > 0 && et.Associazioni[i-1].AllegatoID.String() >= a.AllegatoID.String() {
			e("esito.associazioni non in ordine di allegato")
		}
		prodottiNoti(a.Proposta, "associazione")
		prodottiNoti(a.Pertinente, "associazione")
		prodottiNoti(a.PertinenzaContesto, "associazione")
		if a.Contesto != nil {
			prodottiNoti(a.Contesto.Prodotti, "contesto")
		}
		if a.DocumentoID != nil && docs[*a.DocumentoID] == nil {
			e("associazione di %s: documento %s assente", a.AllegatoID, a.DocumentoID)
		}
		if a.Confermata != nil {
			comp(*a.Confermata, "associazione confermata")
		}
	}
	somma := 0
	for i, f := range cf.File {
		nBadge[f.File.AllegatoID]++
		if i > 0 && cf.File[i-1].File.AllegatoID.String() >= f.File.AllegatoID.String() {
			e("confronto.file non in ordine di allegato")
		}
	}
	for _, n := range cf.Conteggi {
		somma += n
	}
	if len(cf.Conteggi) != 5 || somma != len(cf.File) || cf.VersioneConfronto != confronto.VersioneConfronto {
		e("conteggi %v per %d file", cf.Conteggi, len(cf.File))
	}
	for _, m := range []map[uuid.UUID]int{nFile, nAssoc, nBadge} {
		for a, n := range m {
			if allegati[a] == nil || n != 1 {
				e("record dell'allegato %s: %d, allegato nella foto: %v", a, n, allegati[a] != nil)
			}
		}
	}
	for a := range allegati {
		if nFile[a] != 1 || nAssoc[a] != 1 || nBadge[a] != 1 {
			e("allegato %s: file %d, associazioni %d, badge %d (RAC-08)", a, nFile[a], nAssoc[a], nBadge[a])
		}
	}
	for _, d := range et.DaSmistare {
		if a := allegati[d.AllegatoID]; a == nil || a.NomeFile != d.NomeFile {
			e("da smistare %s (%s) senza il suo allegato", d.AllegatoID, d.NomeFile)
		}
		prodottiNoti(d.Prodotti, "da smistare")
		prodottiNoti(d.ProdottiContesto, "da smistare")
	}

	// La foto: ogni record trova quelli che nomina.
	for _, a := range th.Allegati {
		if a.Sha256 != nil && *a.Sha256 != maShaA(a.ID.String()[34:]) {
			e("allegato %s con lo sha %s", a.ID, *a.Sha256)
		}
	}
	for _, d := range th.Documenti {
		if d.ComponenteID != nil {
			comp(*d.ComponenteID, "documento "+d.ID.String())
		}
		for _, a := range d.Allegati {
			if x := allegati[a]; x == nil || shaDi(a) != d.Sha256 || x.NomeFile != d.NomeFile {
				e("documento %s: allegato %s assente o diverso", d.ID, a)
			}
		}
	}
	for _, p := range th.Proposte {
		if allegati[p.AllegatoID] == nil {
			e("proposta %s: allegato %s assente", p.ID, p.AllegatoID)
		}
		if p.ComponenteID != nil {
			comp(*p.ComponenteID, "proposta "+p.ID.String())
		}
	}
	for _, r := range th.RigheComponenteProposta {
		if a := allegati[r.AllegatoID]; a == nil || shaDi(r.AllegatoID) != r.Sha256 || a.NomeFile != r.NomeFile {
			e("riga %s: allegato %s assente o diverso", r.ID, r.AllegatoID)
		}
		if r.ComponenteID != nil {
			comp(*r.ComponenteID, "riga "+r.ID.String())
		}
	}
	for _, r := range th.RigheRelazioneProposta {
		if a := allegati[r.AllegatoID]; a == nil || a.NomeFile != r.NomeFile {
			e("riga di relazione %s %s>%s: allegato assente o diverso", r.AllegatoID, r.PadreChiave, r.FiglioChiave)
		}
	}
	for _, r := range th.Relazioni {
		comp(r.PadreID, "relazione")
		comp(r.FiglioID, "relazione")
	}
	for _, r := range th.RimozioniAperte {
		if docs[r.StepDocumentoID] == nil || relazione(r.PadreID, r.FiglioID) == nil {
			e("rimozione %s %s>%s senza documento o relazione", r.StepDocumentoID, r.PadreID, r.FiglioID)
		}
	}
	for _, d := range th.Deroghe {
		comp(d.ComponenteID, "deroga "+d.ID.String())
	}
	for _, k := range th.Componenti {
		if k.StepStrutturaleID != nil {
			if d := docs[*k.StepStrutturaleID]; d == nil || d.ComponenteID == nil || *d.ComponenteID != k.ID {
				e("componente %s: STEP strutturale %s non suo", k.ID, k.StepStrutturaleID)
			}
		}
	}

	// I prodotti: strutture, nodi, disegni e voci.
	strutturaDi := func(target string, all uuid.UUID, radice string) *ancoraggio.StrutturaProdotto {
		return maTrova(et.Ancoraggi.Strutture, func(s *ancoraggio.StrutturaProdotto) bool {
			return s.Target == target && s.AllegatoID == all && s.Radice == radice
		})
	}
	disegno := func(d *valutazione.Disegno2D, dove string) {
		if d.AllegatoID == nil || allegati[*d.AllegatoID] == nil || shaDi(*d.AllegatoID) != d.Sha256 || allegati[*d.AllegatoID].NomeFile != d.NomeFile {
			e("%s: 2D %s senza il suo allegato", dove, d.NomeFile)
			return
		}
		if d.Anteprima.AllegatoID == nil || *d.Anteprima.AllegatoID != *d.AllegatoID || d.Anteprima.Sha256 != d.Sha256 {
			e("%s: anteprima del 2D %s diversa dal 2D", dove, d.NomeFile)
		}
		if d.DocumentoID != nil {
			if x := docs[*d.DocumentoID]; x == nil || x.Tipo != string(db.TipoDocumentoDisegno2d) || len(x.Allegati) == 0 || x.Allegati[0] != *d.AllegatoID {
				e("%s: 2D %s con un documento che non è il suo", dove, d.NomeFile)
			}
		}
	}
	gruppo := func(g *valutazione.GruppoDisegni2D, dove string) {
		if g == nil {
			return
		}
		if g.Primario != nil {
			disegno(g.Primario, dove)
		}
		for i := range g.Alternativi {
			disegno(&g.Alternativi[i], dove)
		}
	}
	for pi := range et.ProdottiValutati {
		p := &et.ProdottiValutati[pi]
		dove := p.Rif
		if p.ComponenteID != nil {
			comp(*p.ComponenteID, dove)
			if p.Rif != ancoraggio.RifComponente(*p.ComponenteID) {
				e("%s: Rif diverso dal componente %s", dove, p.ComponenteID)
			}
		}
		rifNodi := map[string]bool{}
		nodo := func(n *valutazione.NodoBOM, quale string) {
			rifNodi[n.Nodo.Rif] = true
			if d := n.Nodo.Decisione; d != nil {
				if k := comps[d.ComponenteID]; k == nil || k.Codice != d.Codice {
					e("%s %s: decisione %s assente o con un altro codice", quale, n.Nodo.Rif, d.ComponenteID)
				}
			}
			if r := n.Nodo.RigaLegacy; r != nil {
				if x := righe[r.ID]; x == nil || x.AllegatoID != r.AllegatoID || x.Chiave != r.Chiave || x.Stato != string(db.StatoPropostaAperta) ||
					ancoraggio.RifNodo(x.Sha256, x.Chiave) != n.Nodo.Rif {
					e("%s %s: riga aperta %s assente o diversa", quale, n.Nodo.Rif, r.ID)
				}
			}
			if r := n.Nodo.RigaDecisa; r != nil {
				if x := righe[r.ID]; x == nil || x.Stato != r.Stato || ancoraggio.RifNodo(x.Sha256, x.Chiave) != n.Nodo.Rif ||
					(r.ComponenteID == nil) != (x.ComponenteID == nil) {
					e("%s %s: riga decisa %s assente o diversa", quale, n.Nodo.Rif, r.ID)
				}
			}
			gruppo(&n.Disegni, quale+" "+n.Nodo.Rif)
		}
		if p.Fonte.Riferimento != nil {
			r := p.Fonte.Riferimento
			if x := docs[r.DocumentoID]; x == nil || r.AllegatoID == nil || shaDi(*r.AllegatoID) != r.Sha256 || x.Sha256 != r.Sha256 {
				e("%s: riferimento della fonte senza il suo documento o allegato", dove)
			} else if p.Fonte.Stato == valutazione.FonteConfermata {
				var k *fotorfq.Componente
				if p.ComponenteID != nil {
					k = comps[*p.ComponenteID]
				}
				if k == nil || k.StepStrutturaleID == nil || *k.StepStrutturaleID != r.DocumentoID {
					e("%s: fonte confermata senza lo STEP strutturale del componente", dove)
				}
			}
		}
		for _, k := range p.Fonte.Candidati {
			if k.AllegatoID == nil || allegati[*k.AllegatoID] == nil || shaDi(*k.AllegatoID) != k.Sha256 {
				e("%s: candidato della fonte senza il suo allegato", dove)
			}
			if k.DocumentoID != nil && docs[*k.DocumentoID] == nil {
				e("%s: candidato della fonte con il documento %s assente", dove, k.DocumentoID)
			}
		}
		if len(p.Nodi) > 0 {
			r := p.Fonte.Riferimento
			if r == nil || r.AllegatoID == nil {
				e("%s: BOM di lavoro senza riferimento della fonte", dove)
			} else {
				radice := ancoraggio.RifNodo(r.Sha256, r.Radice)
				if s := strutturaDi(p.Rif, *r.AllegatoID, radice); s == nil || s.Stato != ancoraggio.StatoBOMDiLavoroProposta || s.Sha256 != r.Sha256 {
					e("%s: manca la struttura della BOM di lavoro (RAC-08)", dove)
				}
				if p.Nodi[0].Nodo.Rif != radice {
					e("%s: il primo nodo della BOM non è la radice", dove)
				}
				for i := range p.Nodi {
					n := &p.Nodi[i]
					nodo(n, "nodo")
					if !strings.HasPrefix(n.Nodo.Rif, ancoraggio.RifNodo(r.Sha256, "")) || n.Nodo.AllegatoID != *r.AllegatoID {
						e("%s: nodo %s fuori dal file della BOM", dove, n.Nodo.Rif)
					}
				}
			}
		}
		for _, sc := range p.StruttureCandidate {
			s := strutturaDi(p.Rif, sc.AllegatoID, sc.Radice)
			if s == nil || s.Stato != ancoraggio.StatoStrutturaCandidata || s.Sha256 != sc.Sha256 || s.RadiceDelFile != sc.RadiceDelFile {
				e("%s: manca la struttura di ancoraggio della candidata %s %s (RAC-08)", dove, sc.AllegatoID, sc.Radice)
			}
			if shaDi(sc.AllegatoID) != sc.Sha256 || len(sc.Nodi) == 0 || sc.Nodi[0].Nodo.Rif != sc.Radice {
				e("%s: candidata %s senza il suo allegato o con la radice fuori posto", dove, sc.AllegatoID)
			}
			for i := range sc.Nodi {
				n := &sc.Nodi[i]
				nodo(n, "nodo candidato")
				if !strings.HasPrefix(n.Nodo.Rif, ancoraggio.RifNodo(sc.Sha256, "")) || n.Nodo.AllegatoID != sc.AllegatoID {
					e("%s: nodo %s fuori dal file della candidata", dove, n.Nodo.Rif)
				}
			}
		}
		for i := range p.ConfermatiSenzaSTEP {
			n := &p.ConfermatiSenzaSTEP[i]
			nodo(n, "confermato senza STEP")
			if n.Nodo.Decisione == nil || n.Nodo.Rif != ancoraggio.RifComponente(n.Nodo.Decisione.ComponenteID) || !n.Nodo.SenzaFile {
				e("%s: confermato senza STEP %s fuori forma", dove, n.Nodo.Rif)
				continue
			}
			for _, par := range n.Parentela {
				padre := maComponenteDelPadre(p, par.Padre)
				if padre == nil {
					e("%s: padre %s del confermato %s senza componente", dove, par.Padre, n.Nodo.Rif)
					continue
				}
				if r := relazione(*padre, n.Nodo.Decisione.ComponenteID); !par.Decisa || r == nil || par.Quantita == nil || r.Qta != *par.Quantita {
					e("%s: il confermato %s sotto %s senza la relazione confermata con la stessa quantità", dove, n.Nodo.Rif, par.Padre)
				}
			}
		}
		for i := range p.Documenti.Voci {
			v := &p.Documenti.Voci[i]
			if k := comps[v.ComponenteID]; k == nil || k.Codice != v.Codice || k.Tipo != v.TipoComponente {
				e("%s: voce di %s senza il suo componente", dove, v.ComponenteID)
			}
			if v.DocumentoID != nil {
				if d := docs[*v.DocumentoID]; d == nil || d.ComponenteID == nil || *d.ComponenteID != v.ComponenteID || d.Tipo != v.TipoDocumento ||
					d.StatoNas != v.StatoNas {
					e("%s: voce %s/%s con un documento che non è il suo", dove, v.Codice, v.TipoDocumento)
				}
			}
			if v.PropostaAperta != nil {
				if x := proposte[*v.PropostaAperta]; x == nil || x.Stato != string(db.StatoPropostaAperta) ||
					v.FileCandidato != nil && x.AllegatoID != *v.FileCandidato {
					e("%s: voce %s/%s con la proposta %s assente o non aperta", dove, v.Codice, v.TipoDocumento, v.PropostaAperta)
				}
			}
			if v.FileCandidato != nil && allegati[*v.FileCandidato] == nil {
				e("%s: voce %s/%s con il file candidato assente", dove, v.Codice, v.TipoDocumento)
			}
			if v.DerogaID != nil && maTrova(th.Deroghe, func(d *fotorfq.DerogaFabbisogno) bool {
				return d.ID == *v.DerogaID && d.ComponenteID == v.ComponenteID && d.Tipo == v.TipoDocumento
			}) == nil {
				e("%s: voce %s/%s con la deroga assente", dove, v.Codice, v.TipoDocumento)
			}
			gruppo(v.Disegni, "voce "+v.Codice)
		}
		for i := range p.Documenti.Previsti {
			x := &p.Documenti.Previsti[i]
			gruppo(x.Disegni, "previsto "+x.Nodo)
			if !rifNodi[x.Nodo] && maTrova(th.Componenti, func(k *fotorfq.Componente) bool { return ancoraggio.RifComponente(k.ID) == x.Nodo }) == nil {
				e("%s: previsto %s senza nodo né componente", dove, x.Nodo)
			}
		}
		for _, x := range p.Documenti.NonBloccanti {
			comp(x.ComponenteID, dove+" non bloccante")
		}
	}

	// Le strutture di ancoraggio: di un prodotto valutato, con il loro file; un arco deciso ha la relazione confermata.
	for _, s := range et.Ancoraggi.Strutture {
		if prodotti[s.Target] == nil || shaDi(s.AllegatoID) != s.Sha256 {
			e("struttura %s %s senza prodotto o allegato", s.Target, s.Radice)
		}
		for _, a := range s.Archi {
			if a.Decisione == nil {
				continue
			}
			pc, ep := uuid.Parse(strings.TrimPrefix(a.Decisione.Padre, "componente:"))
			fc, ef := uuid.Parse(strings.TrimPrefix(a.Decisione.Figlio, "componente:"))
			if ep != nil || ef != nil || relazione(pc, fc) == nil || a.Decisione.Quantita == nil || relazione(pc, fc).Qta != *a.Decisione.Quantita {
				e("arco deciso %s>%s senza la relazione confermata", a.Padre, a.Figlio)
			}
		}
	}

	// I conflitti: di un prodotto valutato (o del thread); una rimozione trova la sua riga (la ricerca di Rimozione).
	for _, k := range et.Conflitti {
		if k.Prodotto != "" && prodotti[k.Prodotto] == nil {
			e("conflitto di %q", k.Prodotto)
		}
		if k.EvidenzaProposta.AllegatoID != nil && allegati[*k.EvidenzaProposta.AllegatoID] == nil {
			e("conflitto %s: allegato dell'evidenza assente", k.Rif)
		}
		if k.EvidenzaProposta.DocumentoID != nil && docs[*k.EvidenzaProposta.DocumentoID] == nil {
			e("conflitto %s: documento dell'evidenza assente", k.Rif)
		}
		if k.Tipo == valutazione.ConflittoArco && k.Motivo == valutazione.MotivoConflittoRimozione {
			if maTrova(th.RimozioniAperte, func(r *fotorfq.RimozioneAperta) bool {
				return k.EvidenzaProposta.DocumentoID != nil && r.StepDocumentoID == *k.EvidenzaProposta.DocumentoID &&
					valutazione.RifArco(ancoraggio.RifComponente(r.PadreID), ancoraggio.RifComponente(r.FiglioID)) == k.Rif
			}) == nil {
				e("conflitto di rimozione %s senza la rimozione aperta", k.Rif)
			}
		}
	}

	// Il fascicolo.
	orfani := 0
	for _, d := range et.DaSmistare {
		if d.Orfano {
			orfani++
		}
	}
	if et.Fascicolo.NumeroTarget != len(et.ProdottiValutati) || et.Fascicolo.Orfani != orfani || len(et.Fascicolo.Avvisi) != orfani {
		e("fascicolo: %d target, %d orfani, %d avvisi", et.Fascicolo.NumeroTarget, et.Fascicolo.Orfani, len(et.Fascicolo.Avvisi))
	}
	return out
}

// maComponenteDelPadre: il componente del padre di un confermato senza STEP: «componente:<uuid>», o il nodo di Nodi che
// lo rappresenta (la sua Decisione; la radice della BOM di lavoro vale il componente del target: val/nodi.go:323-325).
func maComponenteDelPadre(p *valutazione.ProdottoValutato, padre string) *uuid.UUID {
	if strings.HasPrefix(padre, "componente:") {
		u, err := uuid.Parse(strings.TrimPrefix(padre, "componente:"))
		if err != nil {
			return nil
		}
		return &u
	}
	for i := range p.Nodi {
		if p.Nodi[i].Nodo.Rif != padre {
			continue
		}
		if p.Nodi[i].Nodo.Decisione != nil {
			return &p.Nodi[i].Nodo.Decisione.ComponenteID
		}
		if i == 0 {
			return p.ComponenteID
		}
	}
	return nil
}
