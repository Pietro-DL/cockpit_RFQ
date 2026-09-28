package fascicolo

// La scena delle prove L1 del flusso ancorato al prodotto (Smistamento F8): uno StatoFlusso costruito a mano,
// con le evidenze gia' normalizzate come le leggerebbe LeggiStatoFlusso. Identificativi deterministici (dal
// nome): due scene con gli stessi file, costruite in un altro ordine, sono lo stesso stato (FP7).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/db"
)

type scena struct {
	t    *testing.T
	s    StatoFlusso
	chi  uuid.UUID
	t0   time.Time
	n    int
	dich []db.ListDichiarazioniRfqRow
}

func nuovaScena(t *testing.T) *scena {
	t.Helper()
	return &scena{t: t, s: StatoFlusso{Motore: classificazione.Compila("ACME", regole.Regole{}), ImprontaRegole: "regole-acme"},
		chi: idDa("utente"), t0: time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)}
}

func idDa(s string) uuid.UUID { return uuid.NewSHA1(uuid.NameSpaceOID, []byte(s)) }

func shaDel(contenuto string) string {
	h := sha256.Sum256([]byte("contenuto:" + contenuto))
	return hex.EncodeToString(h[:])
}

// prodotto e' un codice della richiesta confermato al triage, con il suo finito.
func (sc *scena) prodotto(codice string) db.Componente {
	sc.identificativo(codice, true)
	return sc.componente(codice, db.TipoComponenteFinito, db.OrigineComponenteCodiceRilevato)
}

func (sc *scena) identificativo(codice string, confermato bool) {
	i := db.IdentificativoThread{Codice: codice, Origine: db.OrigineIdentificativoPropostaFamiglia}
	if confermato {
		i.ConfermatoDa = uuid.NullUUID{UUID: sc.chi, Valid: true}
	}
	sc.s.Identificativi = append(sc.s.Identificativi, i)
}

func (sc *scena) componente(codice string, tipo db.TipoComponente, origine db.OrigineComponente) db.Componente {
	c := db.Componente{ComponenteID: idDa("componente:" + codice), Codice: codice, Tipo: tipo, Origine: origine, ConfermatoDa: sc.chi}
	sc.s.Componenti = append(sc.s.Componenti, c)
	return c
}

func (sc *scena) arco(padre, figlio db.Componente, qta int32) {
	sc.arcoDa(padre, figlio, qta, db.OrigineComponenteManuale)
}

// arcoDa e' un arco della working con l'origine data (step: accettato da uno STEP).
func (sc *scena) arcoDa(padre, figlio db.Componente, qta int32, origine db.OrigineComponente) {
	sc.s.Relazioni = append(sc.s.Relazioni, db.ComponenteRelazione{PadreID: padre.ComponenteID, FiglioID: figlio.ComponenteID, Qta: qta,
		Origine: origine, ConfermatoDa: sc.chi})
}

// file aggiunge un file della RFQ con la sua proposta aperta e la valutazione data; restituisce il suo indice.
func (sc *scena) file(nome, sha string, v classificazione.Valutazione) int {
	sc.n++
	ext := strings.ToLower(strings.TrimPrefix(nome[strings.LastIndex(nome, ".")+1:], "."))
	f := FileFlusso{AllegatoID: idDa("allegato:" + nome), Sha: sha, Nome: nome, Estensione: ext, RicevutoIl: sc.t0.Add(time.Duration(sc.n) * time.Minute),
		DelCliente: true, Analisi: AnalisiCorrente, Proposta: &PropostaFile{PropostaID: idDa("proposta:" + nome), Aperta: true, Valutazione: v}}
	sc.s.File = append(sc.s.File, f)
	return len(sc.s.File) - 1
}

// valuta e' la lettura di un file come la scrive l'analisi: il nome, e quello che il worker ha concluso.
func (sc *scena) valuta(nome string, esito *classificazione.Esito, fatti string, radice *classificazione.Radice) classificazione.Valutazione {
	in := classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: nome, Direzione: "entrata", Motore: sc.s.Motore,
		Esito: esito, Radice: radice}
	if fatti != "" {
		in.Fatti = json.RawMessage(fatti)
	}
	return classificazione.Valuta(in)
}

// disegno e' un PDF letto dal worker con i termini del cartiglio: tipo disegno_2d dal contenuto, codice dal
// nome. Senza evidenze dal contenuto del PDF (EvidenzeContenutoPDF, oggi vuota).
func (sc *scena) disegno(nome string) int {
	return sc.file(nome, shaDel(nome), sc.valuta(nome, &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"},
		`{"cartiglio": true, "termini_trovati": ["SCALA"]}`, nil))
}

// capitolato e' un PDF con i termini di un capitolato nel testo: tipo dal contenuto.
func (sc *scena) capitolato(nome string) int {
	return sc.file(nome, shaDel(nome), sc.valuta(nome, &classificazione.Esito{Tipo: "capitolato", Fonte: "cartiglio"},
		`{"termini_trovati": ["REQUISITI", "FORNITURA"]}`, nil))
}

// dalFormato e' un file di cui si conosce solo il formato (un foglio, una mail).
func (sc *scena) dalFormato(nome string) int {
	return sc.file(nome, shaDel(nome), sc.valuta(nome, nil, "", nil))
}

// step aggiunge un file STEP con la sua struttura: le righe che ApplicaStruttura ne avrebbe tratto. nodi:
// "#1=7120001" (codice = id = nome grezzo) o "#1=7120001-01/7120001" (grezzo/codice); archi: "#1>#2*2".
func (sc *scena) step(nome string, nodi, archi []string) int {
	return sc.stepSha(nome, shaDel(nome), nodi, archi)
}

func (sc *scena) stepSha(nome, sha string, nodi, archi []string) int {
	portatore := idDa("allegato:" + nome)
	gia := false
	for _, n := range sc.s.Nodi {
		if n.Sha256 == sha {
			portatore, gia = n.AllegatoID, true
			break
		}
	}
	radice := map[string]bool{}
	var primo string
	for _, n := range nodi {
		k, _, _ := strings.Cut(n, "=")
		radice[k] = true
		if primo == "" {
			primo = k
		}
	}
	for _, a := range archi {
		pf, _, _ := strings.Cut(a, "*")
		_, f, _ := strings.Cut(pf, ">")
		delete(radice, f)
	}
	var codiceRadice string
	if !gia {
		for _, n := range nodi {
			k, v, _ := strings.Cut(n, "=")
			grezzo, codice, ok := strings.Cut(v, "/")
			if !ok {
				codice = grezzo
			}
			sc.s.Nodi = append(sc.s.Nodi, db.ComponenteProposta{PropostaID: idDa("nodo:" + sha + k), AllegatoID: portatore, Sha256: sha,
				Chiave: k, NomeGrezzo: grezzo, IDGrezzo: grezzo, Codice: pgtype.Text{String: codice, Valid: codice != ""},
				Fonte: db.FontePropostaStep, Stato: db.StatoPropostaAperta})
			if radice[k] && len(radice) == 1 {
				codiceRadice = codice
			}
		}
		for _, a := range archi {
			pf, q, _ := strings.Cut(a, "*")
			qta := 1
			if q != "" {
				fmt.Sscan(q, &qta)
			}
			p, f, _ := strings.Cut(pf, ">")
			sc.s.Archi = append(sc.s.Archi, db.RelazioneProposta{AllegatoID: portatore, PadreChiave: p, FiglioChiave: f, Qta: int32(qta),
				Stato: db.StatoPropostaAperta})
		}
	}
	var r *classificazione.Radice
	if codiceRadice != "" {
		r = &classificazione.Radice{Codice: codiceRadice}
	}
	return sc.file(nome, sha, sc.valuta(nome, &classificazione.Esito{Tipo: "cad_3d", Fonte: "step"}, `{"product_step": "x"}`, r))
}

// accetta: una persona ha accettato il nodo del file come il componente c.
func (sc *scena) accetta(nomeFile, chiave string, c db.Componente) {
	sha := sc.s.File[sc.indice(nomeFile)].Sha
	for i, n := range sc.s.Nodi {
		if n.Sha256 == sha && n.Chiave == chiave {
			sc.s.Nodi[i].Stato, sc.s.Nodi[i].ComponenteID = db.StatoPropostaConfermata, uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
			sc.s.Nodi[i].DecisoDa = uuid.NullUUID{UUID: sc.chi, Valid: true}
			return
		}
	}
	sc.t.Fatalf("nessun nodo %s in %s", chiave, nomeFile)
}

// autorizza: il file e' autorizzato per il componente c, con la sorgente chiave (la marcatura di F5b).
func (sc *scena) autorizza(c db.Componente, nomeFile, chiave string) {
	sha := sc.s.File[sc.indice(nomeFile)].Sha
	sc.dich = append(sc.dich, rigaDich(c, sha, chiave, OrigineSmistamento))
	sc.s.Dichiarazioni = ValutaDichiarazioni(sc.dich)
}

func (sc *scena) indice(nome string) int {
	for i, f := range sc.s.File {
		if f.Nome == nome {
			return i
		}
	}
	sc.t.Fatalf("nessun file %s nella scena", nome)
	return -1
}

func (sc *scena) f(nome string) *FileFlusso { return &sc.s.File[sc.indice(nome)] }

// calcola esegue il flusso sullo stato di adesso.
func (sc *scena) calcola() Calcolo {
	sc.s.der = nil
	sc.s.Dichiarazioni = ValutaDichiarazioni(sc.dich)
	return Calcola(&sc.s, AmbitoRfq())
}

func (sc *scena) dest(c Calcolo, nome string) Destinazione {
	sc.t.Helper()
	d, ok := c.Destinazioni[idDa("allegato:"+nome)]
	if !ok {
		sc.t.Fatalf("nessuna destinazione per %s", nome)
	}
	return d
}

// candidati e' la destinazione in una riga: «rango codice regola [bloccato] {discordanze}», per le prove.
func candidati(d Destinazione) string {
	var parti []string
	for _, c := range d.Candidati {
		x := fmt.Sprintf("%d %s %s", c.Rango, c.Codice, c.Regola)
		if c.Codice == "" {
			x = fmt.Sprintf("%d %s %s", c.Rango, c.Chiave, c.Regola)
		}
		if c.Bloccato != "" {
			x += " [" + c.Bloccato + "]"
		}
		parti = append(parti, x)
	}
	return strings.Join(parti, " | ")
}
