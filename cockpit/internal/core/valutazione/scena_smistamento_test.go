// L1 — gli aiuti delle prove dello smistamento (B6, V2): la scena di un prodotto altrimenti verificato (la scena della
// completezza con i 2D, la fonte confermata, la BOM verificata e la vista), il secondo prodotto con il suo STEP non
// autorizzato, i messaggi del thread (con e senza il gesto 1, in uscita, di un fornitore, interni, inoltrati), i file con
// le loro proposte, le letture dell'esito; costruiti in Go come li darebbe il caricatore.
package valutazione_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example), con la famiglia acme-catena (712xxxx con il marcatore A
// o B e la revisione di una cifra); i nomi dei file sono di fantasia, gli UUID sono 00000000-0000-4000-8000-0000000000nn.
// Le prove citano i requisiti (R81, R93, R95 A, R105, R106, R107, R108, T-B0-25, T-B0-29, T-B0-32, T-E1-12, PO-04, PO-13,
// PO-14, PO-15, PO-21, PO-24, PO-25, PO-27, PO-32, PO-39, PO-40), mai i casi degli attesi.

// Gli ID della scena dello smistamento: il secondo finito e il suo STEP, i messaggi in più, i file e le proposte.
var (
	cP2       = uid(0xa31)
	aStepP2   = uid(0xa32)
	rifP2     = "componente:" + uid(0xa31).String()
	mDue      = uid(0xa01) // nomina il prodotto e il secondo prodotto
	mNessuno  = uid(0xa02) // non nomina nessun prodotto
	mSenza    = uid(0xa03) // nomina il secondo prodotto, senza il gesto 1
	mUscita   = uid(0xa04) // in uscita verso un fornitore
	mFornit   = uid(0xa05) // in entrata da un fornitore
	mInterno  = uid(0xa06) // un inoltro interno, in uscita fra colleghi
	mAltroCl  = uid(0xa07) // in entrata da un altro cliente
	mInoltro  = uid(0xa08) // un inoltro senza confine: tutto il corpo è storia (R48 A)
	mSoloP2   = uid(0xa09) // nomina solo il secondo prodotto
	idFornito = uuid.MustParse("00000000-0000-4000-8000-00000000f001")
)

// shaN: uno sha256 di prova dal numero (64 cifre esadecimali), per i file della scena dello smistamento.
func shaN(n int) string { return fmt.Sprintf("%064x", 0x5000+n) }

// scenaSmistamento: il prodotto 7120100A altrimenti verificato (fonte confermata, BOM verificata con «Conferma
// l'albero», i 2D del finito e dello sciolto confermati, la completezza completa): lo smistamento è verificato, perché
// ogni file è terminale e nessun file è pertinente senza decisione.
func scenaSmistamento(t *testing.T) fotorfq.Thread {
	t.Helper()
	return scenaCompletezza(t, true)
}

// secondoProdotto: il finito manuale 7120900A (target, R70 A) con il suo STEP fra gli allegati del primo messaggio (la
// radice 7120900A e il figlio 7120910A, più gli altri figli dati, grafo completo), non autorizzato: la sua struttura è
// candidata. Un figlio in comune con il primo prodotto (7120200A) è un figlio condiviso (FIGLIO-CONDIVISO).
func secondoProdotto(t *testing.T, th *fotorfq.Thread, altriFigli ...string) {
	t.Helper()
	th.Componenti = append(th.Componenti, componente(cP2, "7120900A", "finito", "manuale"))
	nodi, archi := []nodoF{{"#1", "7120900A", ""}, {"#2", "7120910A", ""}}, []arcoF{{"#1", "#2", 1}}
	for i, c := range altriFigli {
		k := fmt.Sprintf("#%d", i+3)
		nodi, archi = append(nodi, nodoF{k, c, ""}), append(archi, arcoF{"#1", k, 1})
	}
	f := fattiStepF(t, shaN(1), nodi, archi)
	conAllegato(th, allegato(aStepP2, 9, "7120900A_1.stp", "stp", shaN(1)), &f)
	th.StepProdotto = append(th.StepProdotto, fotorfq.RigaStepProdotto{ComponenteID: cP2, Codice: "7120900A", Esito: "da_scegliere"})
}

// conMessaggio: un messaggio in entrata dal cliente ACME, con il gesto 1 dell'operatore se conGesto (altrimenti
// agganciato in automatico), l'oggetto e il corpo dati.
func conMessaggio(th *fotorfq.Thread, id uuid.UUID, minuti int, oggetto, corpo string, conGesto bool) *fotorfq.Messaggio {
	th.Messaggi = append(th.Messaggi, messaggio(id, minuti, oggetto, corpo))
	g := fotorfq.AggancioMessaggio{MessaggioID: id, Aggancio: "auto_conversazione"}
	if conGesto {
		g = gesto(id)
	}
	th.Agganci = append(th.Agganci, g)
	return &th.Messaggi[len(th.Messaggi)-1]
}

// fileNelMessaggio: un allegato «file» del messaggio dato, con nome, estensione e sha, e i suoi fatti se ci sono.
func fileNelMessaggio(th *fotorfq.Thread, id, msg uuid.UUID, nome, est, s string, f *fotorfq.Fatti) {
	a := allegato(id, int16(40+len(th.Allegati)), nome, est, s)
	a.MessaggioID = msg
	conAllegato(th, a, f)
}

// conProposta: la proposta di documento_proposta del file: il tipo, la fonte, lo stato, il componente («assegna» su una
// proposta aperta) e chi l'ha decisa.
func conProposta(th *fotorfq.Thread, id, allegato uuid.UUID, tipo, fonte, stato string, comp, decisoDa *uuid.UUID) *fotorfq.PropostaAttuale {
	p := fotorfq.PropostaAttuale{ID: id, AllegatoID: allegato, Tipo: tipo, Fonte: fonte, Stato: stato, ComponenteID: comp, DecisoDa: decisoDa}
	if decisoDa != nil {
		il := dataACME.Add(3 * time.Hour)
		p.DecisoIl = &il
	}
	th.Proposte = append(th.Proposte, p)
	return &th.Proposte[len(th.Proposte)-1]
}

// destinazione: i dettagli di una proposta con la destinazione F8 (le chiavi dei candidati, nell'ordine dato).
func destinazione(t *testing.T, chiavi ...string) json.RawMessage {
	t.Helper()
	var c []map[string]any
	for _, k := range chiavi {
		c = append(c, map[string]any{"chiave": k})
	}
	raw, err := json.Marshal(map[string]any{"destinazione": map[string]any{"candidati": c}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// esitoSmistamento: Calcola sulla fotografia del thread (la vista calcolata come la calcola v_fascicolo, l'insieme delle regole
// ACME, i casi dati), con gli invarianti dell'aiuto calcola; il thread ACME dell'esito.
func esitoSmistamento(t *testing.T, th fotorfq.Thread, casi ...valutazione.IngressoCaso) valutazione.EsitoThread {
	t.Helper()
	return esitoSmistamentoCon(t, th, insiemeACME(t), casi...)
}

// esitoSmistamentoCon: come esitoSmistamento, con l'insieme delle regole dato.
func esitoSmistamentoCon(t *testing.T, th fotorfq.Thread, r *motorea.InsiemeRegole, casi ...valutazione.IngressoCaso) valutazione.EsitoThread {
	t.Helper()
	vistaCome(&th)
	in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: casi}
	return esitoThread(t, calcola(t, fotografia(th), r, in), threadACME)
}

// insiemeConStoria: l'insieme delle regole ACME con la famiglia della catena che legge i codici anche nella storia della
// mail (la forma «mail» e la sua revisione anche sul selettore storia), come la famiglia con la P di fase (R48 A: la
// storia si legge, e non si promuove senza il caso).
func insiemeConStoria(t *testing.T) *motorea.InsiemeRegole {
	t.Helper()
	f := famigliaCatena()
	for i := range f.Forme {
		if f.Forme[i].ID == "mail" {
			f.Forme[i].Selettori = []string{"oggetto", "corpo", "storia"}
		}
	}
	f.Revisioni[0].Selettori = append(f.Revisioni[0].Selettori, "storia")
	return insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", f)})
}

// ---- le letture dell'esito ----

func associazioneDi(t *testing.T, et valutazione.EsitoThread, allegato uuid.UUID) valutazione.AssociazioneFile {
	t.Helper()
	for _, a := range et.Associazioni {
		if a.AllegatoID == allegato {
			return a
		}
	}
	t.Fatalf("nessuna associazione per l'allegato %s", allegato)
	return valutazione.AssociazioneFile{}
}

func daSmistareDi(et valutazione.EsitoThread, allegato uuid.UUID) (valutazione.FileDaSmistare, bool) {
	for _, d := range et.DaSmistare {
		if d.AllegatoID == allegato {
			return d, true
		}
	}
	return valutazione.FileDaSmistare{}, false
}

func prodottoDi(t *testing.T, et valutazione.EsitoThread, rif string) valutazione.ProdottoValutato {
	t.Helper()
	for _, p := range et.ProdottiValutati {
		if p.Rif == rif {
			return p
		}
	}
	t.Fatalf("il prodotto %s non c'è nell'esito", rif)
	return valutazione.ProdottoValutato{}
}

// nonTerminale: il file è fra i file non terminali dello smistamento del prodotto.
func nonTerminale(s valutazione.VerificaSmistamento, allegato uuid.UUID) bool {
	for _, a := range s.FileNonTerminali {
		if a == allegato {
			return true
		}
	}
	return false
}

// motiviDi: i motivi dello smistamento come testo, per i confronti.
func motiviDi(s valutazione.VerificaSmistamento) string {
	out := ""
	for i, m := range s.Motivi {
		if i > 0 {
			out += " "
		}
		out += string(m)
	}
	return out
}

// conflittiDi: i conflitti dell'esito del tipo dato.
func conflittiDi(et valutazione.EsitoThread, tipo valutazione.TipoConflitto) []valutazione.Conflitto {
	var out []valutazione.Conflitto
	for _, c := range et.Conflitti {
		if c.Tipo == tipo {
			out = append(out, c)
		}
	}
	return out
}

// pdfSenzaCodice: un PDF scansionato, senza testo e con un nome senza codice: nessuna identità letta.
func pdfSenzaCodice(t *testing.T, th *fotorfq.Thread, id, msg uuid.UUID, nome string, n int) {
	t.Helper()
	fileNelMessaggio(th, id, msg, nome, "pdf", shaN(n), scansione(t, shaN(n)))
}
