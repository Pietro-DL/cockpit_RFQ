package web

// Il banco finto della Distinta unica di V1 (giro 5, A1d, V1; direttiva 23 §7: «La UI può procedere su dati di prova
// conformi al contratto mentre il backend viene completato, ma questi dati non costituiscono una prova di
// integrazione»). È un httptest.Server senza database che serve:
//   - il guscio VERO della pagina (paginaDistintaUnica, distinta_unica_pagina.go) con gli statici veri dell'embed;
//     l'admin si mette nel contesto come lo mette autenticato (nessuna sessione: nessun database);
//   - risposte FINTE, in memoria, delle rotte di V1 che le corsie persistenza, file e motore stanno scrivendo: il modello
//     /motore-a/dati con le aggiunte di V1, R1-R7, R11, il ramo JSON di /thread/{id}/carica (E1′), S1-S7, l'anteprima
//     dell'autorizzazione e i sei atti della lista chiusa ridotta. Le forme sono quelle degli esempi dell'emendamento di
//     V1 (EM §9 e §10.9, con il C3 §4.2 e §5.5 dove EM rimanda); dove EM non dà una forma, quella proposta nel resoconto
//     della corsia, detta nei commenti;
//   - una sonda (/__banco/…) per la prova nel browser: lo stato del banco (scritture, corpi, chiavi, chiamate legacy) e
//     gli esiti da provocare una volta (409, 422, 5xx, la risposta persa, il motore che non risponde).
//
// Il banco NON è il server: non riconcilia le azioni, non calcola il motore, non ha le regole di dominio. Fa quel poco
// che serve perché i flussi della matrice si vedano nel browser (versioni della bozza, chiavi d'idempotenza, stati dei
// file, un riepilogo con i blocchi più semplici). Le regole vere le provano le L4 delle altre corsie.
//
// Solo dati inventati: ACME (il cliente), BETA (il partner), codici ACM-xxxx e 400123, UUID 00000000-0000-4000-8000-…

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// I dati del banco: un cliente, un partner, due prodotti, due STEP, tre PDF, un DXF, un file interno, uno zip con due
// voci.
const (
	bdThread   = "00000000-0000-4000-8000-000000000100"
	bdCliente  = "00000000-0000-4000-8000-0000000c1001"
	bdP1       = "00000000-0000-4000-8000-000000001000"
	bdP2       = "00000000-0000-4000-8000-000000002000"
	bdA1       = "00000000-0000-4000-8000-0000000a0001" // STEP della BOM
	bdA2       = "00000000-0000-4000-8000-0000000a0002" // PDF ACM-1100 rev A (proposto per il telaio)
	bdA3       = "00000000-0000-4000-8000-0000000a0003" // PDF ACM-1120 rev B (rev diversa: conflitto)
	bdA4       = "00000000-0000-4000-8000-0000000a0004" // PDF senza codice
	bdA5       = "00000000-0000-4000-8000-0000000a0005" // DXF per «Altri file»
	bdA6       = "00000000-0000-4000-8000-0000000a0006" // file interno caricato a mano: unica copia (D-14)
	bdA7       = "00000000-0000-4000-8000-0000000a0007" // zip con due voci
	bdA8       = "00000000-0000-4000-8000-0000000a0008" // voce 1 dello zip
	bdA9       = "00000000-0000-4000-8000-0000000a0009" // voce 2 dello zip
	bdA10      = "00000000-0000-4000-8000-0000000a000a" // secondo STEP (cambio della fonte)
	bdA11      = "00000000-0000-4000-8000-0000000a000b" // PDF associato al componente registrato ACM-1400 (AR)
	bdA12      = "00000000-0000-4000-8000-0000000a000c" // PDF di ACM-1400 già documento confermato
	bdA13      = "00000000-0000-4000-8000-0000000a000d" // secondo zip con due voci (Altri file e Riattiva)
	bdA14      = "00000000-0000-4000-8000-0000000a000e" // voce 1 del secondo zip
	bdA15      = "00000000-0000-4000-8000-0000000a000f" // voce 2 del secondo zip
	bdC40      = "00000000-0000-4000-8000-000000004000" // il componente registrato ACM-1400 (un registrato che non è la radice)
	bdShaStep  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bdShaStep2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func bdProposta(allegato string) string {
	return "00000000-0000-4000-8000-0000000d" + allegato[len(allegato)-4:]
}
func bdGruppo(allegato, sha string) string {
	return "cand:" + allegato + ":nodo:" + sha + ":#1"
}
func bdSha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

type m = map[string]any

// fileBanco: un'occorrenza della Sorgente (EM §10.1: RFQ + allegato_id).
type fileBanco struct {
	ID, Proposta, Nome, Est, Sha, Origine, Canale, Stato, Contenitore string
	Versione                                                          int
	Analisi                                                           string
	Rimozione, Conservazione, Scarto, AltriFile, Documento            m
	Codice, Rev, Componente                                           string
	letture                                                           int
}

// bozzaBanco: la bozza aperta di un prodotto (EM §9.1, §9.2). Formato2: una bozza del formato di prima, che R1 dice
// da convertire (EM §6.7).
type bozzaBanco struct {
	ID, Stato, Registro, ChiaveUltimo string
	Versione                          int
	Snapshot                          m
	Eventi                            []m
	Creata                            time.Time
	Formato2                          bool
	// Base: la versione 1 di una bozza aperta dalla conferma o dalla BOM registrata, cioè la conferma di base come la
	// costruisce il server; con l'interruttore uguale-alla-base R4 confronta la bozza con questa (PE-23)
	Base m
}

type intentoBanco struct {
	Gesto, Impronta, Risorsa string
	Risposta                 m
	Creato                   time.Time
}

type bancoDU struct {
	t                *testing.T
	s                *Server
	srv              *httptest.Server
	mu               sync.Mutex
	file             []*fileBanco
	bozze            map[string]*bozzaBanco
	conferme         map[string][]m
	intenti          map[string]intentoBanco
	salvati          map[string]string // chiave_salvataggio → impronta del corpo
	risposte         map[string]m      // chiave_salvataggio → risposta registrata
	registri         map[string]m      // impronta → contenuto del registro d'importazione
	modi             map[string]string // rotta → esito da provocare una volta
	fonte            m
	conti            map[string]int // scritture per rotta
	corpiR2          []m
	chiaviR5         []string
	chiaviR6         []string
	corpiFile        []m
	legacy           []string
	nBozze           int                      // le bozze create: il numero entra nel bozza_id, che non si ripete
	ritardi          map[string]time.Duration // rotta (il pattern del mux) → attesa prima della risposta, una volta
	f8Errore         bool                     // il riallineamento dei documenti dopo l'ultima conferma è in errore, finché R11 non lo rifà
	versioneProposta int                      // cambia l'impronta di oggi del gruppo (la «proposta cambiata»)
	congelata        bool                     // la BOM del thread è congelata (fascicolo.congelato di /dati)
	s6Incerta        bool                     // S6 dà una sola candidata, incerta: nessuna in vista finché non la si sceglie
	baseSuperata     bool                     // R1 dice la base superata, finché un salvataggio con «rebase» non la porta alla corrente
	// segnoUguale: R4 dà uguale_alla_base e R5 rifiuta con NESSUNA_MODIFICA una bozza uguale alla conferma di base
	// formato 3, come farà il server dal passo 6 (decisioni dell'ondata A, PE-23); spento, il banco è il server di oggi,
	// che non lo dice, e la pagina decide dal giornale
	segnoUguale bool
	// radiceSegnalata: la voce della radice del registro ha il segnale LS-02 (il caso di P-6 del motore: la regola resta
	// legata al file, e il nodo del prodotto può portarlo)
	radiceSegnalata bool
}

// nuovoBancoDU: il banco, con i dati di partenza. Il server del Cockpit è quello delle prove (template e statici
// dell'embed, nessun database) con il flag [motore_a] acceso.
func nuovoBancoDU(t *testing.T) *bancoDU {
	t.Helper()
	b := &bancoDU{t: t, s: duServer(t, true), bozze: map[string]*bozzaBanco{}, conferme: map[string][]m{}, intenti: map[string]intentoBanco{},
		salvati: map[string]string{}, risposte: map[string]m{}, registri: map[string]m{}, modi: map[string]string{}, conti: map[string]int{}, ritardi: map[string]time.Duration{},
		corpiR2: []m{}, chiaviR5: []string{}, chiaviR6: []string{}, corpiFile: []m{}, legacy: []string{}}
	b.partenza()
	mux := http.NewServeMux()
	b.monta(mux)
	b.srv = httptest.NewServer(mux)
	t.Cleanup(b.srv.Close)
	return b
}

func (b *bancoDU) partenza() {
	nuovo := func(id, nome, est, sha, origine, canale, codice, rev string) *fileBanco {
		return &fileBanco{ID: id, Proposta: bdProposta(id), Nome: nome, Est: est, Sha: sha, Origine: origine, Canale: canale, Stato: "attivo", Analisi: "letto", Codice: codice, Rev: rev}
	}
	b.file = []*fileBanco{
		nuovo(bdA1, "ACM-1000_assieme.stp", "stp", bdShaStep, "portale", "portale", "ACM-1000", "C"),
		nuovo(bdA2, "ACM-1100_A.pdf", "pdf", bdSha("a2"), "portale", "posta", "ACM-1100", "A"),
		nuovo(bdA3, "ACM-1120_B.pdf", "pdf", bdSha("a3"), "portale", "posta", "ACM-1120", "B"),
		nuovo(bdA4, "SCHEMA_GENERALE.pdf", "pdf", bdSha("a4"), "portale", "posta", "", ""),
		nuovo(bdA5, "PIASTRA_400123.dxf", "dxf", bdSha("a5"), "portale", "posta", "400123", ""),
		nuovo(bdA6, "collaudo_interno.pdf", "pdf", bdSha("a6"), "interna", "caricamento", "", ""),
		nuovo(bdA7, "pacchetto.zip", "zip", bdSha("a7"), "portale", "portale", "", ""),
		nuovo(bdA8, "voce_1.pdf", "pdf", bdSha("a8"), "portale", "portale", "", ""),
		nuovo(bdA9, "voce_2.pdf", "pdf", bdSha("a9"), "portale", "portale", "", ""),
		nuovo(bdA10, "ACM-1000_rev2.stp", "stp", bdShaStep2, "portale", "portale", "ACM-1000", "D"),
		nuovo(bdA11, "ACM-1400_A.pdf", "pdf", bdSha("a11"), "portale", "posta", "ACM-1400", "A"),
		nuovo(bdA12, "ACM-1400_ASSIEME.pdf", "pdf", bdSha("a12"), "portale", "posta", "ACM-1400", "A"),
		nuovo(bdA13, "pacchetto2.zip", "zip", bdSha("a13"), "portale", "portale", "", ""),
		nuovo(bdA14, "voce_3.pdf", "pdf", bdSha("a14"), "portale", "portale", "", ""),
		nuovo(bdA15, "voce_4.pdf", "pdf", bdSha("a15"), "portale", "portale", "", ""),
	}
	b.file[7].Contenitore, b.file[8].Contenitore = bdA7, bdA7
	b.file[13].Contenitore, b.file[14].Contenitore = bdA13, bdA13
	// le associazioni registrate (AR) del componente ACM-1400: una aperta, una già documento confermato (MAT §4.3 riga 3)
	b.file[10].Componente, b.file[11].Componente = bdC40, bdC40
	b.file[11].Documento = m{"documento_id": "00000000-0000-4000-8000-0000000f0d12", "stato": "confermato", "confermato_il": "2026-10-10T09:00:00Z"}
	// lo STEP secondario della fonte (EM §9.9): la forma è quella proposta nel resoconto della corsia
	b.fonte = m{"voce": bdProposta(bdA1), "documento_id": "00000000-0000-4000-8000-0000000f0d01", "sha256": bdShaStep, "conferma_ammessa": false,
		"motivo": "lo STEP non è associato al prodotto: associalo dalla Sorgente", "confermata": false, "autorizzata": false,
		"step_secondario": m{"allegato_id": bdA10, "nome_file": "ACM-1000_rev2.stp", "motivo": "corrispondenza esatta e univoca"}}
}

func (b *bancoDU) fileDi(id string) *fileBanco {
	for _, f := range b.file {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// ---------------------------------------------------------------- il registro d'importazione e lo stato di partenza

// lettura: una lettura compatta del registro (EM §3.8, «Una lettura»), con i soli campi che la pagina mostra.
func bdLettura(allegato, id, letterale, emittente, ns string) m {
	return m{"chiave_lettura": allegato + "/" + id, "unita_id": "u:" + id, "testo_unita": letterale, "intervallo": m{"inizio": 0, "fine": len(letterale)},
		"letterale": letterale, "emittente": emittente, "hash_grammatica": strings.Repeat("7", 64), "namespace": ns, "famiglia": "codice", "forma": "base",
		"base": m{"originale": letterale, "normalizzata": letterale, "completa": true}, "qualificatori": m{"affissi": []any{}}, "revisione": nil, "stato_forma": "letta", "ruolo": "primaria"}
}

// voceBanco: una voce di nodo del registro (EM §3.8, «Voce di un nodo»).
func bdVoce(allegato, sha, chiave, nome, codice, rev, tipo, emittente, ns string, padri []string, foglia bool) m {
	rif := "nodo:" + sha + ":" + chiave
	v := m{"voce": "n:" + allegato + ":" + rif, "origine": m{"allegato_id": allegato, "sha256": sha, "chiave": chiave, "rif": rif}, "entita_id": "e" + chiave,
		"padri": padri, "foglia": foglia, "radice_del_file": len(padri) == 0, "leggibile": codice != "", "stato_motore": m{"raggiunto": true, "tolto": false, "deciso": false},
		"grezzi": m{"id": codice, "nome": nome, "descrizione": nome}, "codice_proposto": codice, "rev_proposta": rev, "codice_composto": m{"testo": codice, "motivo": ""},
		"alternative": []any{}, "riferimenti": []any{}, "attributi": []any{}, "tipo_proposto": m{"valore": tipo, "origine": "regola"}, "segnali": []any{},
		"diagnostiche": []any{}, "associazioni_proposte": []any{}, "fabbisogni_previsti_all_importazione": []any{}}
	if codice != "" {
		v["identita_proposta"] = bdLettura(allegato, "l:"+chiave, codice, emittente, ns)
	} else {
		v["identita_proposta"] = nil
		v["motivo_proposto"] = "nessuna_lettura"
		v["motivo_lettura"] = "nessuna_lettura"
	}
	return v
}

// registro: il contenuto del registro d'importazione di uno STEP (EM §3.8), sintetico. Il gruppo è la candidata
// dello STEP. versione cambia l'impronta (la proposta di oggi diversa da quella importata).
func (b *bancoDU) registro(allegato, sha string, versione int) m {
	cli := "cliente:" + bdCliente
	// i contro-indizi del segnale sono codici dell'ancoraggio (SegnaleNodo.ControIndizi), assenti senza
	saldatura := func(chiave, n string, controIndizi ...string) m {
		v := bdVoce(allegato, sha, chiave, "ACME_111_2.000_Saldatura d'angolo "+n+"_oa_0", "", "", "sciolto", cli, "", []string{"#7"}, true)
		segnale := m{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1",
			"riscontri":   []any{m{"campo": "nome", "unita_id": "u:n" + chiave, "parole": []any{"Saldatura"}, "intervalli": []any{m{"inizio": 15, "fine": 24}}}},
			"spiegazione": "Il nome del pezzo nel file contiene «Saldatura»; il nodo non ha figli e non è una radice del file: possibile elemento CAD, non un articolo. È solo un segnale: il nodo resta nella proposta e il suo 2D resta richiesto finché non decidi."}
		if len(controIndizi) > 0 {
			segnale["contro_indizi"] = controIndizi
		}
		v["segnali"] = []any{segnale}
		v["fabbisogni_previsti_all_importazione"] = []any{m{"tipo_documento": "disegno_2d", "bloccante": true}}
		return v
	}
	nodi := []any{
		bdVoce(allegato, sha, "#1", "ACM-1000 Confezionatrice", "ACM-1000", "C", "finito", cli, "acme", nil, false),
		bdVoce(allegato, sha, "#7", "Telaio saldato", "ACM-1100", "A", "sottoassieme", cli, "acme", []string{"#1"}, false),
		bdVoce(allegato, sha, "#9", "Staffa del fornitore", "400123", "", "sciolto", "partner:beta", "beta", []string{"#7"}, true),
		bdVoce(allegato, sha, "#11", "Traversa", "ACM-1120", "A", "sciolto", cli, "acme", []string{"#7"}, true),
		bdVoce(allegato, sha, "#12", "Piastrina", "", "", "sciolto", cli, "", []string{"#7"}, true),
		saldatura("#31", "1"), saldatura("#32", "2"), saldatura("#33", "3", "grafo_non_completo"),
		bdVoce(allegato, sha, "#40", "WELDED SUPPORT", "ACM-1400", "A", "sottoassieme", cli, "acme", []string{"#1"}, false),
		bdVoce(allegato, sha, "#41", "Piede", "ACM-1410", "A", "sciolto", cli, "acme", []string{"#40"}, true),
		bdVoce(allegato, sha, "#50", "Coperchio", "ACM-1500", "A", "sciolto", cli, "acme", []string{"#1"}, true),
	}
	nodi[10].(m)["stato_motore"] = m{"raggiunto": true, "tolto": true, "deciso": false}
	nodi[1].(m)["associazioni_proposte"] = []any{m{"allegato_id": bdA2, "motivo": "codice e rev nel nome del file", "livello": "forte"}}
	nodi[3].(m)["associazioni_proposte"] = []any{m{"allegato_id": bdA3, "motivo": "codice nel nome del file", "livello": "medio"}}
	// la lettura con ruolo «riferimento» porta la proiezione del codice e la rev proposta (decisioni dell'ondata A,
	// PE-08): «Adotta come codice…» la cita
	letturaDxf := bdLettura(bdA5, "l:dxf", "400123", "partner:beta", "beta")
	letturaDxf["codice_proposto"], letturaDxf["rev_proposta"] = "400123", "B"
	nodi[2].(m)["riferimenti"] = []any{m{"tipo": "documentale", "valore": "400123", "esito": "concorde", "fonte": "cartiglio"},
		m{"tipo": "lettura_ruolo", "ruolo": "riferimento", "lettura": letturaDxf, "fonte": "PIASTRA_400123.dxf"}}
	// il componente registrato della riga legacy decisa (P-3, EM §6.1): ACM-1400 è un registrato che non è la radice
	nodi[8].(m)["decisione_esistente"] = m{"componente_id": bdC40, "codice": "ACM-1400", "rev": "A"}
	// il P/N equivalente K4 di un PDF del fornitore (EM §3.8, riferimenti[]): l'avviso EQUIVALENTE_NON_ADOTTATO finché una
	// decisione non lo adotta o lo rifiuta (EM §3.6.1)
	nodi[9].(m)["riferimenti"] = []any{m{"tipo": "equivalente_k4", "equivalente": m{"namespace": "beta", "base": "400777", "emittente": "partner:beta",
		"prove": []any{m{"allegato_id": bdA2, "entita": "e1", "lettura_codice": "l:pdf:pn", "lettura_riferimento": "l:pdf:rif", "fonte": "cartiglio"}}}, "fonte": "ACM-1100_A.pdf"}}
	// il codice corretto a mano nella pagina vecchia: solo evidenza, si adotta con una decisione (EM §3.8, §3.6.1)
	nodi[3].(m)["codice_manuale_legacy"] = m{"codice": "ACM-1125", "rev": "A", "riga_legacy": "00000000-0000-4000-8000-0000000c0011"}
	if versione > 0 {
		nodi[3].(m)["codice_proposto"] = "ACM-1121"
		nodi[3].(m)["identita_proposta"] = bdLettura(allegato, "l:#11", "ACM-1121", cli, "acme")
	}
	if b.radiceSegnalata {
		nodi[0].(m)["segnali"] = bdSegnaleRadice()
	}
	archi := []any{}
	for _, x := range [][3]any{{"#1", "#7", 1}, {"#7", "#9", 2}, {"#7", "#11", 4}, {"#7", "#12", 2}, {"#7", "#31", 1}, {"#7", "#32", 1}, {"#7", "#33", 1}, {"#1", "#40", 1}, {"#40", "#41", 2}, {"#1", "#50", 1}} {
		archi = append(archi, m{"allegato_id": allegato, "padre": "nodo:" + sha + ":" + x[0].(string), "figlio": "nodo:" + sha + ":" + x[1].(string), "quantita": x[2], "occorrenze": []any{}})
	}
	gruppo := bdGruppo(allegato, sha)
	contenuto := m{"formato_registro": 1, "versione_importazione": "importazione-1", "thread_id": bdThread, "prodotto_id": bdP1,
		"gruppo": m{"chiave": gruppo, "tipo": "cand", "allegato_id": allegato, "sha256": sha, "radice": "nodo:" + sha + ":#1", "grafo_completo": true},
		"regole": m{"versioni": m{"ancoraggio": "ancoraggio-3", "importazione": "importazione-1"}, "grammatica_cliente": m{"cliente_id": bdCliente, "hash": strings.Repeat("7", 64)},
			"partner": []any{m{"emittente": "partner:beta", "hash": strings.Repeat("8", 64)}}},
		"contesto": nil,
		"proposta": m{"fonte": m{"allegato_id": allegato, "sha256": sha, "nome_ricevuto": b.fileDi(allegato).Nome}, "nodi": nodi, "archi": archi,
			"relazioni": []any{m{"voce": "r:1", "id_motore": "rel-1", "tipo": "documentato_da", "stato": "proposta", "sorgente": m{"tipo": "nodo", "voce": nodi[2].(m)["voce"]},
				"lettura_sorgente": allegato + "/l:#9", "letture_bersaglio": []any{bdA5 + "/l:dxf"}, "evidenze": []any{"u:dxf"}}},
			"file": []any{}, "fuori_per_decisione": []any{m{"voce": nodi[10].(m)["voce"], "segno": "tolto", "autore": "AD", "motivo": "coperchio fornito a parte"}}, "diagnostiche": []any{}},
		"allegati_citati": []any{allegato, bdA2, bdA3, bdA5}}
	contenuto["impronta_contenuto"] = bdSha(fmt.Sprintf("contenuto|%s|%d", gruppo, versione))
	contenuto["impronta_evidenze"] = bdSha("evidenze|" + gruppo)
	contenuto["impronta_regole"] = bdSha("regole|1")
	return contenuto
}

// bdSegnaleRadice: il segnale LS-02 sulla voce della radice (interruttore radice-segnalata; P-6 del motore).
func bdSegnaleRadice() []any {
	return []any{m{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1",
		"spiegazione": "Segnale di prova sul nodo del prodotto: la regola resta legata al file (P-6)."}}
}

// impronta: la chiave del registro (sha256 del contenuto intero; qui un'impronta sintetica stabile).
func bdImpronta(reg m) string {
	return bdSha("registro|" + reg["impronta_contenuto"].(string))
}

// statoDiPartenza: le regole P-1…P-5 della partenza «step» (EM §6.1) applicate al registro sintetico, come le
// applicherebbe il server (il banco le imita solo per avere una bozza da mostrare).
func (b *bancoDU) statoDiPartenza(reg m, impronta string) m {
	prop := reg["proposta"].(m)
	gruppo := reg["gruppo"].(m)["chiave"].(string)
	allegato := reg["gruppo"].(m)["allegato_id"].(string)
	sha := reg["gruppo"].(m)["sha256"].(string)
	chiave := func(rif string) string { return "k-" + strings.TrimPrefix(rif[strings.LastIndex(rif, ":")+1:], "#") }
	nodi := []any{}
	inclusi := map[string]bool{}
	for _, x := range prop["nodi"].([]any) {
		v := x.(m)
		o := v["origine"].(m)
		if sm, _ := v["stato_motore"].(m); sm["tolto"] == true || sm["raggiunto"] == false {
			continue
		}
		inclusi[bdStr(o["rif"])] = true
		casella := m{"allegato_id": o["allegato_id"], "gruppo": gruppo, "rif": o["rif"]}
		k := chiave(o["rif"].(string))
		if o["chiave"] == "#1" {
			nodi = append(nodi, m{"chiave_bozza": "k-root", "componente_id": bdP1, "op": "riusa", "tipo": "finito", "descrizione": "Confezionatrice MX-1", "origine": "registrato",
				"caselle": []any{casella}, "identita": m{"emittente": "cliente:" + bdCliente, "emittente_presunto": true, "codice": "ACM-1000", "rev": "C", "modo": "registrato"}})
			continue
		}
		if de, _ := v["decisione_esistente"].(m); de != nil && bdStr(de["componente_id"]) != bdP1 {
			// P-3: la voce con un componente registrato è un riuso, con l'identità registrata (cliente, presunto)
			nodi = append(nodi, m{"chiave_bozza": k, "componente_id": de["componente_id"], "op": "riusa", "tipo": v["tipo_proposto"].(m)["valore"], "descrizione": v["grezzi"].(m)["nome"],
				"origine": "step", "caselle": []any{casella}, "identita": m{"emittente": "cliente:" + bdCliente, "emittente_presunto": true, "codice": de["codice"], "rev": de["rev"], "modo": "registrato"}})
			continue
		}
		id := m{}
		if cp, _ := v["codice_proposto"].(string); cp != "" {
			ip := v["identita_proposta"].(m)
			id = m{"emittente": ip["emittente"], "namespace": ip["namespace"], "codice": cp, "modo": "proposta"}
			if rp, _ := v["rev_proposta"].(string); rp != "" {
				id["rev"] = rp
			}
		} else {
			id = m{"emittente": "cliente:" + bdCliente, "modo": "da_decidere"}
		}
		nodi = append(nodi, m{"chiave_bozza": k, "op": "crea", "tipo": v["tipo_proposto"].(m)["valore"], "descrizione": v["grezzi"].(m)["nome"], "origine": "step",
			"caselle": []any{casella}, "identita": id})
	}
	archi := []any{}
	for _, x := range prop["archi"].([]any) {
		a := x.(m)
		if !inclusi[bdStr(a["padre"])] || !inclusi[bdStr(a["figlio"])] {
			continue
		}
		p, f := chiave(a["padre"].(string)), chiave(a["figlio"].(string))
		if p == "k-1" {
			p = "k-root"
		}
		archi = append(archi, m{"padre": p, "figlio": f, "qta": a["quantita"], "origine": "step", "step": m{"allegato_id": allegato, "gruppo": gruppo}})
	}
	return m{"formato": 3, "prodotto_id": bdP1, "sorgente": m{"allegato_id": allegato, "sha256": sha, "gruppo": gruppo, "importazione": impronta},
		"versione_bom_base": m{"conferma_id": nil, "numero": 0, "contatore": 12}, "firma_base": strings.Repeat("1", 64),
		"nodi": nodi, "archi": archi, "decisioni": []any{}, "documenti_preassociati": []any{}}
}

// ---------------------------------------------------------------- il montaggio

var bdLegacy = regexp.MustCompile(`/distinta/|/fascicolo/nodo/|/fascicolo/rimozione/|/fascicolo/assegna|/proposta/|/smistamento/aggiorna|/fascicolo/componente/[^/]+/(codice|modifica|tipo|collega|sposta|scollega|archivia|rimuovi|ripristina|deroga)|/fascicolo/(documento|deroga)/`)

func (b *bancoDU) monta(mux *http.ServeMux) {
	mux.Handle("GET /static/", http.StripPrefix("/static/", b.s.statici()))
	p := b.s.nuovaPaginaDistintaUnica(func(_ context.Context, id uuid.UUID) (testataDistintaUnica, error) {
		if id.String() != bdThread {
			return testataDistintaUnica{}, errRFQDistintaUnica
		}
		return testataDistintaUnica{Oggetto: "RFQ ACME-0418 confezionatrice", Riferimento: "ACME-0418", Stato: "APERTA"}, nil
	})
	admin := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			u := &db.Utente{UtenteID: duUUID(77), Sigla: "AD", Nome: "Amministratore di prova", Ruolo: db.RuoloUtenteAdmin}
			h(w, r.WithContext(context.WithValue(r.Context(), ctxUtente, u)))
		}
	}
	mux.HandleFunc("GET "+percorsoDistintaUnica, b.s.conMotoreA(conNoStore(admin(b.s.soloRuolo(db.RuoloUtenteAdmin, p.servi)))))
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "login") })
	// l'uscita del layout (il form «esci»): la prova nel browser guarda che cosa resta nella memoria della scheda
	// (richiesta R-3 della corsia); il banco non ha sessioni, rimanda solo al login
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/login", http.StatusSeeOther) })
	// il poll dello stato dei worker è del layout (frammenti.html, stato_worker), non della pagina: il banco risponde con
	// lo span vuoto, senza hx-get, e il poll si ferma
	mux.HandleFunc("GET /stato/worker", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<span id="stato-worker" class="stato-worker"></span>`)
	})
	r := func(pattern string, h func(w http.ResponseWriter, r *http.Request)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
			if req.PathValue("id") != "" && req.PathValue("id") != bdThread {
				bdJSON(w, http.StatusNotFound, m{"esito": "rifiutato", "codice": "RFQ_INESISTENTE", "testo": "RFQ non trovata"})
				return
			}
			// un'attesa provocata dalla sonda (lo stato «Salvataggio…» della pagina): fuori dal lucchetto, perché le
			// altre rotte rispondano intanto
			b.mu.Lock()
			attesa := b.ritardi[pattern]
			delete(b.ritardi, pattern)
			b.mu.Unlock()
			if attesa > 0 {
				time.Sleep(attesa)
			}
			// il corpo del caricamento si legge prima del lucchetto: un caricamento lento (la prova del tempo d'inattività
			// di E1′) non deve fermare le altre rotte mentre arriva. e1 trova il modulo già letto
			if pattern == "POST /thread/{id}/carica" && strings.Contains(req.Header.Get("Accept"), "application/json") {
				_ = req.ParseMultipartForm(8 << 20)
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			h(w, req)
		})
	}
	r("GET /thread/{id}/motore-a/dati", b.dati)
	r("GET /thread/{id}/motore-a/anteprima/step-strutturale", b.anteprimaAutorizzazione)
	r("GET /thread/{id}/motore-a/bozza", b.r1)
	r("POST /thread/{id}/motore-a/bozza", b.r2)
	r("POST /thread/{id}/motore-a/bozza/{b}/abbandona", b.r3)
	r("POST /thread/{id}/motore-a/bom/riepilogo", b.r4)
	r("POST /thread/{id}/motore-a/bom/conferma", b.r5)
	r("GET /thread/{id}/motore-a/intento/{k}", b.r6)
	r("GET /thread/{id}/motore-a/conferme", b.r7)
	r("POST /thread/{id}/motore-a/riallinea", b.r11)
	r("POST /thread/{id}/carica", b.e1)
	r("GET /thread/{id}/sorgente", b.s1)
	r("POST /thread/{id}/sorgente/file/{a}/scarta", b.s2)
	r("POST /thread/{id}/sorgente/file/{a}/altri-file", b.s3)
	r("POST /thread/{id}/sorgente/file/{a}/riattiva", b.s4)
	r("GET /thread/{id}/sorgente/file/{a}/contenuto", b.s5)
	r("GET /thread/{id}/sorgente/step/{a}/anteprima", b.s6)
	r("POST /thread/{id}/sorgente/step/{a}/associa", b.s7)
	for _, atto := range []string{"/fascicolo/conferma", "/fascicolo/componente/{u}/step-strutturale", "/fascicolo/componente/{u}/step-strutturale/revoca",
		"/fascicolo/prepara", "/fascicolo/rianalizza", "/riprova-copie"} {
		r("POST /thread/{id}"+atto, b.atto)
	}
	mux.HandleFunc("/__banco/", b.sonda)
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if bdLegacy.MatchString(req.URL.Path) {
			b.mu.Lock()
			b.legacy = append(b.legacy, req.Method+" "+req.URL.Path)
			b.mu.Unlock()
		}
		http.NotFound(w, req)
	})
}

func bdJSON(w http.ResponseWriter, stato int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(stato)
	_ = json.NewEncoder(w).Encode(v)
}

func bdCorpo(r *http.Request) (m, []byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return nil, nil, err
	}
	var o m
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&o); err != nil {
		return nil, raw, err
	}
	return o, raw, nil
}

// modo: l'esito da provocare per la rotta, una volta sola.
func (b *bancoDU) modo(rotta string) string {
	v := b.modi[rotta]
	delete(b.modi, rotta)
	return v
}

func bdKo(w http.ResponseWriter, stato int, codice, testo string, extra m) {
	o := m{"esito": map[int]string{409: "conflitto", 422: "rifiutato", 410: "rifiutato", 503: "rifiutato", 504: "rifiutato"}[stato], "codice": codice, "testo": testo}
	for k, v := range extra {
		o[k] = v
	}
	bdJSON(w, stato, o)
}

// ---------------------------------------------------------------- il modello della pagina

func (b *bancoDU) dati(w http.ResponseWriter, r *http.Request) {
	if b.modo("dati") == "errore" {
		bdJSON(w, http.StatusOK, m{"thread_id": bdThread, "vista": "distinta", "prodotto": "", "testa": m{"cliente": "ACME Macchine"}, "esito": nil, "confronto": nil,
			"errore": "Il calcolo del motore A non è riuscito: il dettaglio è nel log del server.", "calcolato": time.Now().UTC()})
		return
	}
	scelto := r.URL.Query().Get("prodotto")
	if scelto == "" {
		scelto = "prodotto:ACM-1000"
	}
	prodotti := []any{
		m{"rif": "prodotto:ACM-1000", "componente_id": bdP1, "codice_richiesto": "ACM-1000", "descrizione": "Confezionatrice MX-1", "prodotto_verificato": false, "motivi": []any{m{"codice": "bom_non_confermata"}}},
		m{"rif": "prodotto:ACM-2000", "componente_id": bdP2, "codice_richiesto": "ACM-2000", "descrizione": "Nastro di carico", "prodotto_verificato": false, "motivi": []any{}},
	}
	o := m{"thread_id": bdThread, "vista": "distinta", "prodotto": scelto, "testa": m{"cliente": "ACME Macchine", "oggetto": "RFQ ACME-0418", "riferimento_cliente": "ACME-0418", "stato_thread": "APERTA"},
		"esito":     m{"thread": []any{m{"thread_id": bdThread, "cliente_id": bdCliente, "valutato": true, "prodotti_valutati": prodotti, "fascicolo": m{"congelato": b.congelata}}}},
		"confronto": m{}, "errore": "", "calcolato": time.Now().UTC(), "f8_stato": map[bool]string{false: "eseguito", true: "errore"}[b.f8Errore]}
	if scelto == "prodotto:ACM-1000" {
		gruppi := []any{}
		for _, a := range []string{bdA1, bdA10} {
			f := b.fileDi(a)
			if f == nil || f.Stato != "attivo" {
				continue
			}
			reg := b.registro(a, f.Sha, b.versioneProposta)
			// per la fascia «Quale struttura guardare» (MAT SG-01): radice letta, compatibilità con il prodotto, stato della
			// struttura (LS-01) e nodi segnalati (LS-02), con i nomi proposti nel resoconto della corsia (PE-01)
			letta, comp := "ACM-1000", "compatibile"
			if a == bdA10 {
				letta = "ACM-1000-R2"
			}
			if b.s6Incerta && a == bdA1 {
				comp = "incerta"
			}
			gruppi = append(gruppi, m{"chiave": bdGruppo(a, f.Sha), "tipo": "cand", "allegato_id": a, "radice": "nodo:" + f.Sha + ":#1", "radice_letterale": letta, "compatibilita": comp,
				"struttura": m{"versione": 3, "completa": true, "motivo": ""}, "segnalati": 3, "nome_file": f.Nome, "impronta_contenuto": reg["impronta_contenuto"]})
		}
		o["gruppi"] = gruppi
		o["fonte"] = b.fonte
		if bz := b.bozze[bdP1]; bz != nil && bz.Stato == "aperta" && bz.Registro != "" {
			reg := b.registri[bz.Registro]
			gc := m{"chiave": reg["gruppo"].(m)["chiave"], "come": "stessa_occorrenza"}
			if f := b.fileDi(reg["gruppo"].(m)["allegato_id"].(string)); f == nil || f.Stato != "attivo" {
				gc = m{"come": "assente", "motivo": "occorrenza_non_attiva", "allegato_id": reg["gruppo"].(m)["allegato_id"]}
			}
			o["bozza_del_prodotto"] = m{"bozza_id": bz.ID, "versione": bz.Versione, "gruppo_corrente": gc}
		}
	} else {
		o["gruppi"] = []any{}
		o["fonte"] = m{"conferma_ammessa": false, "motivo": "il prodotto non ha uno STEP associato", "confermata": false, "autorizzata": false}
	}
	bdJSON(w, http.StatusOK, o)
}

func (b *bancoDU) anteprimaAutorizzazione(w http.ResponseWriter, r *http.Request) {
	if !b.fonte["confermata"].(bool) {
		bdJSON(w, http.StatusOK, m{"motore": "fascicolo_legacy", "errore": "la fonte non è confermata: prima «Conferma la fonte»", "frasi": []any{}, "figli": []any{}, "sorgenti": []any{},
			"raggruppamenti": []any{}, "deleghe": []any{}, "revoche": []any{}, "avvisi": []any{}})
		return
	}
	bdJSON(w, http.StatusOK, m{"motore": "fascicolo_legacy", "componente": m{"id": bdP1, "codice": "ACM-1000"}, "allegato": bdA1, "sha256": bdShaStep, "nome_file": "ACM-1000_assieme.stp",
		"documento_id": b.fonte["documento_id"], "autorizzabile": true, "inviabile": true, "frasi": []any{"Lo STEP ACM-1000_assieme.stp diventa lo STEP strutturale di ACM-1000.", "I suoi figli diretti entrano nell'autorità della struttura."},
		"figli": []any{m{"chiave": "#7", "codice": "ACM-1100", "qta": 1}, m{"chiave": "#40", "codice": "ACM-1400", "qta": 1}}, "sorgenti": []any{}, "raggruppamenti": []any{}, "deleghe": []any{},
		"revoche": []any{}, "avvisi": []any{}, "presa_d_atto": "Ho letto l'anteprima: l'autorizzazione rilegge lo STEP e non conferma la BOM.", "bottone": "Autorizza lo STEP strutturale",
		"firma": strings.Repeat("f", 64)})
}

// ---------------------------------------------------------------- R1-R7, R11

func bdVersioneCorrente(conf []m) m {
	if len(conf) == 0 {
		return m{"conferma_id": nil, "numero": 0, "contatore": 12, "firma_struttura": strings.Repeat("1", 64), "fonte_autorizzata": nil}
	}
	u := conf[len(conf)-1]
	return m{"conferma_id": u["conferma_id"], "numero": u["numero"], "contatore": 13, "firma_struttura": strings.Repeat("3", 64), "fonte_autorizzata": nil}
}

func (b *bancoDU) r1(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	prod := q.Get("prodotto")
	if prod != bdP1 && prod != bdP2 {
		bdKo(w, 422, "FUORI_RFQ", "il prodotto non è di questa RFQ", nil)
		return
	}
	if c := q.Get("effetto_cambio_tipo"); c != "" {
		b.effettoCambioTipo(w, c, q.Get("tipo_nuovo"))
		return
	}
	bz := b.bozze[prod]
	if bz == nil || bz.Stato != "aperta" {
		bdJSON(w, http.StatusOK, m{"stato": "nessuna_bozza", "versione_corrente": bdVersioneCorrente(b.conferme[prod])})
		return
	}
	if bz.Formato2 {
		// una bozza del formato di prima: R1 la dà com'era, con la conversione necessaria (EM §9.1, §6.7)
		bdJSON(w, http.StatusOK, m{"stato": "aperta", "formato": 2, "vocabolario": 1, "bozza_id": bz.ID, "versione": bz.Versione, "snapshot": bz.Snapshot,
			"impronta": bdSha(bdCanonico(bz.Snapshot)), "conversione": m{"necessaria": true, "forma": "converti_da"}, "creata_il": bz.Creata, "aggiornata_il": bz.Creata,
			"eventi": len(bz.Eventi), "versione_corrente": bdVersioneCorrente(b.conferme[prod]), "base": m{"stato": "valida", "motivi": []any{}}, "rif_scaduti": []any{}, "vassoio": []any{}})
		return
	}
	base := m{"stato": "valida", "motivi": []any{}}
	if b.baseSuperata {
		base = m{"stato": "superata", "motivi": []any{"conferma_cambiata"}}
	}
	o := m{"stato": "aperta", "formato": 3, "vocabolario": 2, "bozza_id": bz.ID, "versione": bz.Versione, "snapshot": bz.Snapshot, "impronta": bdSha(bdCanonico(bz.Snapshot)),
		"chiave_ultimo_salvataggio": bz.ChiaveUltimo, "creata_da": duUUID(77).String(), "creata_il": bz.Creata, "aggiornata_da": duUUID(77).String(), "aggiornata_il": time.Now().UTC(),
		"eventi": len(bz.Eventi), "versione_corrente": bdVersioneCorrente(b.conferme[prod]), "base": base, "rif_scaduti": []any{}, "vassoio": []any{},
		"importazioni_citate": []any{}}
	if bz.Registro != "" {
		reg := b.registri[bz.Registro]
		imp := m{"importazione_id": "00000000-0000-4000-8000-0000000b1001", "impronta": bz.Registro, "impronta_contenuto": reg["impronta_contenuto"], "impronta_evidenze": reg["impronta_evidenze"],
			"impronta_regole": reg["impronta_regole"], "gruppo": reg["gruppo"].(m)["chiave"], "creata_da": duUUID(77).String(), "creata_il": bz.Creata, "regole": reg["regole"]}
		if q.Get("importazione") == "1" {
			imp["contenuto"] = reg
		}
		o["importazione"] = imp
		o["importazioni_citate"] = []any{m{"impronta": bz.Registro, "creata_il": bz.Creata}}
	}
	if q.Get("eventi") == "1" {
		o["eventi"] = bz.Eventi
	}
	if q.Get("confronto") == "1" && bz.Registro != "" {
		reg := b.registri[bz.Registro]
		oggi := b.registro(reg["gruppo"].(m)["allegato_id"].(string), reg["gruppo"].(m)["sha256"].(string), b.versioneProposta)
		c := m{"gruppo": m{"chiave": reg["gruppo"].(m)["chiave"], "come": "stessa_occorrenza"}, "impronta_contenuto_corrente": oggi["impronta_contenuto"], "differenze": m{"nodi": []any{}, "archi": []any{}}}
		if oggi["impronta_contenuto"] == reg["impronta_contenuto"] {
			c["esito"] = "uguale"
		} else {
			c["esito"] = "proposta_cambiata"
			c["differenze"] = m{"nodi": []any{m{"chiave_bozza": "k-11", "voce": "n:" + bdA1 + ":nodo:" + bdShaStep + ":#11", "tipo": "cambiato", "campi": []any{"identita_proposta", "codice_proposto"}}}, "archi": []any{}}
		}
		o["confronto"] = c
	}
	bdJSON(w, http.StatusOK, o)
}

// effettoCambioTipo: la variante di lettura di R1 per l'effetto del cambio di tipo di un componente registrato
// (decisioni dell'ondata A, PE-09): {componente_id, tipo_nuovo, effetto, firma_effetto}, senza scrivere. La forma di
// «effetto» è quella proposta nel resoconto della corsia (i campi dell'anteprima del cambio di tipo del fascicolo, in
// minuscolo). Il banco conosce solo il componente registrato ACM-1400, un assieme con un figlio nella BOM registrata:
// diventa un particolare con un avviso; un commerciale no (ha figli, E37); lo stesso tipo no.
func (b *bancoDU) effettoCambioTipo(w http.ResponseWriter, componente, tipo string) {
	if _, err := uuid.Parse(componente); err != nil || !db.TipoComponente(tipo).Valid() {
		bdKo(w, 422, "CORPO_NON_VALIDO", "componente o tipo nuovo non validi", m{"campo": "effetto_cambio_tipo"})
		return
	}
	if componente != bdC40 {
		bdKo(w, 422, "FUORI_RFQ", "il componente non è di questa RFQ", nil)
		return
	}
	b.conti["R1_effetto_tipo"]++
	effetto := m{"spento": "", "sospende": []any{}, "con_lei": []any{}, "rimozioni": 0, "resta_sospesa": []any{}, "avvisi": []any{}}
	switch tipo {
	case "sottoassieme":
		effetto["spento"] = "ACM-1400 è già un assieme"
	case "commerciale":
		effetto["spento"] = "ACM-1400 ha un figlio nella BOM registrata: un commerciale non ha figli (E37)"
	case "finito":
		effetto["spento"] = "finito solo sul prodotto della RFQ"
	default:
		effetto["avvisi"] = []any{"ACM-1400 diventa un particolare: il suo figlio nella BOM va ricollocato, o diventa assieme (D1)."}
		effetto["rimozioni"] = 1
	}
	bdJSON(w, http.StatusOK, m{"componente_id": componente, "tipo_nuovo": tipo, "effetto": effetto, "firma_effetto": bdSha("effetto|" + componente + "|" + tipo)[:32]})
}

func bdNum(v any) int {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case float64:
		return int(x)
	case int:
		return x
	}
	return -1
}
func bdStr(v any) string { s, _ := v.(string); return s }

func (b *bancoDU) eventoServer(bz *bozzaBanco, azione string, dopo m) {
	n := len(bz.Eventi) + 1
	bz.Eventi = append(bz.Eventi, m{"n": n, "versione": bz.Versione, "vocabolario": 2, "azione": azione, "chiave": nil, "prima": nil, "dopo": dopo, "origine": "server", "motivo": nil,
		"autore": "AD", "quando": time.Now().UTC()})
}

// r2: le cinque forme di R2 (EM §9.2). L'ordine dei controlli è quello del contratto: la chiave_salvataggio prima
// della versione attesa e della bozza aperta.
func (b *bancoDU) r2(w http.ResponseWriter, r *http.Request) {
	corpo, _, err := bdCorpo(r)
	if err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il corpo non è JSON", m{"campo": "corpo"})
		return
	}
	chiave := bdStr(corpo["chiave_salvataggio"])
	if !regexp.MustCompile(`^[0-9a-f-]{36}$`).MatchString(chiave) || bdNum(corpo["vocabolario"]) != 2 {
		bdKo(w, 422, "CORPO_NON_VALIDO", "chiave_salvataggio o vocabolario non validi", m{"campo": "chiave_salvataggio"})
		return
	}
	senzaChiave := m{}
	for k, v := range corpo {
		if k != "chiave_salvataggio" {
			senzaChiave[k] = v
		}
	}
	impronta := bdSha(bdCanonico(senzaChiave))
	if prima, ok := b.salvati[chiave]; ok {
		if prima != impronta {
			bdKo(w, 409, "CHIAVE_RIUSATA", "la chiave del salvataggio ha già un corpo diverso", nil)
			return
		}
		rr := b.risposte[chiave]
		rr["riproposto"] = true
		bdJSON(w, http.StatusOK, rr)
		return
	}
	prod := bdStr(corpo["prodotto_id"])
	if prod != bdP1 && prod != bdP2 {
		bdKo(w, 422, "FUORI_RFQ", "il prodotto non è di questa RFQ", m{"campo": "prodotto_id"})
		return
	}
	modo := b.modo("R2")
	switch modo {
	case "409":
		bz := b.bozze[prod]
		bdKo(w, 409, "BOZZA_VERSIONE", "La bozza è cambiata altrove (AD, alle "+time.Now().Format("15:04")+").", m{"motivo": "versione_diversa", "versione": bz.Versione})
		return
	case "422":
		bdKo(w, 422, "CORPO_NON_VALIDO", "Le azioni non riconciliano lo snapshot.", m{"motivo": "azioni_non_riconciliate", "campo": "azioni"})
		return
	case "500":
		bdJSON(w, http.StatusInternalServerError, m{"esito": "da_verificare", "testo": "errore interno"})
		return
	case "503":
		bdKo(w, 503, "CALCOLO_OCCUPATO", "Il motore A sta già facendo altri calcoli: riprovare fra poco.", nil)
		return
	}
	b.corpiR2 = append(b.corpiR2, corpo)
	var risposta m
	switch {
	case corpo["crea"] != nil:
		risposta = b.r2Crea(w, corpo, prod)
	case corpo["cambia_sorgente"] != nil:
		risposta = b.r2CambiaSorgente(w, corpo, prod)
	case corpo["riallinea_proposta"] != nil:
		risposta = b.r2Riallinea(w, corpo, prod)
	case corpo["converti_da"] != nil:
		risposta = b.r2Converti(w, corpo, prod)
	default:
		risposta = b.r2Salva(w, corpo, prod)
	}
	if risposta == nil {
		return
	}
	b.conti["R2"]++ // le scritture riuscite della bozza
	b.salvati[chiave] = impronta
	b.risposte[chiave] = risposta
	b.bozze[prod].ChiaveUltimo = chiave
	if modo == "500_dopo" {
		bdJSON(w, http.StatusInternalServerError, m{"esito": "da_verificare", "testo": "errore dopo la scrittura"})
		return
	}
	bdJSON(w, http.StatusOK, risposta)
}

func (b *bancoDU) r2Crea(w http.ResponseWriter, corpo m, prod string) m {
	crea := corpo["crea"].(m)
	partenza := bdStr(crea["partenza"])
	abbandona, _ := crea["abbandona"].(m)
	vecchia := b.bozze[prod]
	if vecchia != nil && vecchia.Stato == "aperta" {
		if abbandona == nil {
			bdKo(w, 409, "BOZZA_VERSIONE", "c'è già una bozza aperta per il prodotto", m{"motivo": "altra_bozza_aperta", "bozza_id": vecchia.ID})
			return nil
		}
		if bdStr(abbandona["bozza_id"]) != vecchia.ID || bdNum(abbandona["versione_attesa"]) != vecchia.Versione {
			bdKo(w, 409, "BOZZA_VERSIONE", "La bozza da abbandonare è cambiata.", m{"motivo": "versione_diversa", "versione": vecchia.Versione})
			return nil
		}
		if partenza != "step" || bdStr(abbandona["motivo"]) == "" {
			bdKo(w, 422, "CORPO_NON_VALIDO", "la ripartenza è dalla proposta, con il motivo", m{"motivo": "partenza_non_ammessa"})
			return nil
		}
	} else if abbandona != nil {
		bdKo(w, 409, "BOZZA_VERSIONE", "la bozza da abbandonare è chiusa", m{"motivo": "bozza_chiusa"})
		return nil
	}
	b.nBozze++
	bz := &bozzaBanco{ID: fmt.Sprintf("00000000-0000-4000-8000-0000000b%04d", b.nBozze), Stato: "aperta", Versione: 1, Creata: time.Now().UTC()}
	switch partenza {
	case "step":
		sorg, _ := crea["sorgente"].(m)
		a := bdStr(sorg["allegato_id"])
		f := b.fileDi(a)
		if f == nil || f.Stato != "attivo" {
			bdKo(w, 422, "FILE_NON_ATTIVO", "lo STEP non è fra i file attivi", nil)
			return nil
		}
		reg := b.registro(a, f.Sha, b.versioneProposta)
		if bdStr(crea["impronta_contenuto"]) != reg["impronta_contenuto"] {
			bdKo(w, 409, "ANTEPRIMA_SUPERATA", "La proposta è cambiata mentre guardavi: rivedila.", m{"anteprima": "proposta_step", "impronta_contenuto": reg["impronta_contenuto"]})
			return nil
		}
		if a == bdA10 && bdStr(crea["motivo"]) == "" {
			bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "Il codice letto dalla radice dello STEP non è quello del prodotto: serve un motivo.", m{"campo": "crea.motivo"})
			return nil
		}
		imp := bdImpronta(reg)
		b.registri[imp] = reg
		bz.Registro = imp
		bz.Snapshot = bdSnapshotCanonico(b.statoDiPartenza(reg, imp))
		if abbandona != nil && vecchia != nil {
			vecchia.Stato = "abbandonata"
			b.conti["ripartenze"]++
		}
		b.bozze[prod] = bz
		b.eventoServer(bz, "bozza_aperta", m{"partenza": "step", "importazione": imp})
		b.eventoServer(bz, "sorgente_scelta", m{"allegato_id": a, "gruppo": reg["gruppo"].(m)["chiave"]})
		return m{"bozza_id": bz.ID, "formato": 3, "versione": 1, "nuova_versione": true, "riproposto": false, "eventi": 2, "impronta": bdSha(bdCanonico(bz.Snapshot)), "salvata_il": time.Now().UTC(),
			"snapshot": bz.Snapshot, "importazione": m{"importazione_id": "00000000-0000-4000-8000-0000000b1001", "impronta": imp, "impronta_contenuto": reg["impronta_contenuto"],
				"impronta_evidenze": reg["impronta_evidenze"], "riusata": false}}
	case "conferma", "registrato":
		if partenza == "conferma" && len(b.conferme[prod]) == 0 {
			bdKo(w, 422, "CORPO_NON_VALIDO", "il prodotto non ha una conferma", m{"motivo": "partenza_non_ammessa"})
			return nil
		}
		b.costruisciDaRegistrato(bz, prod, partenza)
		b.bozze[prod] = bz
		b.eventoServer(bz, "bozza_aperta", m{"partenza": partenza})
		return m{"bozza_id": bz.ID, "formato": 3, "versione": 1, "nuova_versione": true, "riproposto": false, "eventi": 1, "impronta": bdSha(bdCanonico(bz.Snapshot)), "salvata_il": time.Now().UTC(), "snapshot": bz.Snapshot}
	}
	bdKo(w, 422, "CORPO_NON_VALIDO", "partenza non ammessa", m{"motivo": "partenza_non_ammessa"})
	return nil
}

// costruisciDaRegistrato: la versione 1 delle partenze «conferma» (da-conferma-1, EM §6.12) e «registrato», come la
// costruirebbe il server: le identità registrate, la base all'ultima conferma. Il banco mette sotto la radice del primo
// prodotto, dalla conferma, il componente registrato ACM-1400 con l'arco della conferma. La versione 1 resta anche come
// Base: con l'interruttore uguale-alla-base R4 e R5 confrontano la bozza con lei (PE-23).
func (b *bancoDU) costruisciDaRegistrato(bz *bozzaBanco, prod, partenza string) {
	radice := m{"chiave_bozza": "k-root", "componente_id": prod, "op": "riusa", "tipo": "finito", "origine": "registrato",
		"identita": m{"emittente": "cliente:" + bdCliente, "emittente_presunto": true, "codice": map[string]string{bdP1: "ACM-1000", bdP2: "ACM-2000"}[prod], "modo": "registrato"}}
	nodi, archi := []any{radice}, []any{}
	base := m{"conferma_id": nil, "numero": len(b.conferme[prod]), "contatore": 13}
	if n := len(b.conferme[prod]); n > 0 {
		base["conferma_id"] = b.conferme[prod][n-1]["conferma_id"]
	}
	if partenza == "conferma" {
		radice["origine"] = "conferma"
		if prod == bdP1 {
			nodi = append(nodi, m{"chiave_bozza": "k-40", "componente_id": bdC40, "op": "riusa", "tipo": "sottoassieme", "descrizione": "Supporto saldato", "origine": "conferma",
				"identita": m{"emittente": "cliente:" + bdCliente, "emittente_presunto": true, "codice": "ACM-1400", "rev": "A", "modo": "registrato"}})
			archi = append(archi, m{"padre": "k-root", "figlio": "k-40", "qta": 1, "origine": "conferma"})
		}
	}
	bz.Snapshot = bdSnapshotCanonico(m{"formato": 3, "prodotto_id": prod, "versione_bom_base": base, "firma_base": strings.Repeat("3", 64),
		"nodi": nodi, "archi": archi, "decisioni": []any{}, "documenti_preassociati": []any{}})
	bz.Base = bdSnapshotCanonico(bz.Snapshot)
}

// ugualeAllaBase: la bozza (lo snapshot del corpo di R4 o R5) è uguale alla sua conferma di base formato 3 (PE-23). Il
// banco confronta con la versione 1 delle partenze dalla conferma o dalla BOM registrata (Base); una bozza nata da uno
// STEP non è mai uguale. ok = falso se il banco non lo dice (interruttore spento).
func (b *bancoDU) ugualeAllaBase(prod string, snap m) (uguale, ok bool) {
	if !b.segnoUguale {
		return false, false
	}
	bz := b.bozze[prod]
	if bz == nil || bz.Base == nil {
		return false, true
	}
	numero := bdNum(bz.Base["versione_bom_base"].(m)["numero"])
	f3 := false
	for _, c := range b.conferme[prod] {
		if bdNum(c["numero"]) == numero && bdNum(c["formato"]) == 3 && c["valida"] == true {
			f3 = true
		}
	}
	return f3 && bdCanonico(bdFormaCanonica(snap, "")) == bdCanonico(bdFormaCanonica(bz.Base, "")), true
}

func (b *bancoDU) bozzaAttesa(w http.ResponseWriter, corpo m, prod string) *bozzaBanco {
	bz := b.bozze[prod]
	if bz == nil || bz.Stato != "aperta" {
		bdKo(w, 409, "BOZZA_VERSIONE", "la bozza è chiusa", m{"motivo": "bozza_chiusa"})
		return nil
	}
	if bdStr(corpo["bozza_id"]) != bz.ID || bdNum(corpo["versione_attesa"]) != bz.Versione {
		bdKo(w, 409, "BOZZA_VERSIONE", "La bozza è cambiata altrove.", m{"motivo": "versione_diversa", "versione": bz.Versione})
		return nil
	}
	return bz
}

func (b *bancoDU) r2Salva(w http.ResponseWriter, corpo m, prod string) m {
	bz := b.bozzaAttesa(w, corpo, prod)
	if bz == nil {
		return nil
	}
	snap, ok := corpo["snapshot"].(m)
	azioni, ok2 := corpo["azioni"].([]any)
	if !ok || !ok2 || len(azioni) == 0 {
		bdKo(w, 422, "CORPO_NON_VALIDO", "servono lo snapshot e le azioni", m{"campo": "azioni"})
		return nil
	}
	bz.Versione++
	// il server salva lo snapshot nella forma canonica (EM §3.10): elenchi ordinati, facoltativi vuoti tolti; R1 restituisce
	// questi byte
	bz.Snapshot = bdSnapshotCanonico(snap)
	if corpo["rebase"] != nil {
		// il rebase porta la base alla corrente (EM §9.2): la base non è più superata
		b.baseSuperata = false
	}
	for _, x := range azioni {
		a := x.(m)
		n := len(bz.Eventi) + 1
		bz.Eventi = append(bz.Eventi, m{"n": n, "versione": bz.Versione, "vocabolario": 2, "azione": a["azione"], "chiave": a["chiave_bozza"], "prima": a["prima"], "dopo": a["dopo"],
			"origine": a["origine"], "motivo": a["motivo"], "autore": "AD", "quando": time.Now().UTC()})
	}
	return m{"bozza_id": bz.ID, "formato": 3, "versione": bz.Versione, "nuova_versione": true, "riproposto": false, "eventi": len(azioni), "impronta": bdSha(bdCanonico(bz.Snapshot)), "salvata_il": time.Now().UTC()}
}

// r2Converti: la conversione di una bozza del formato di prima (EM §6.7, forma converti_da): la vecchia si chiude, la
// nuova (formato 3, senza registro) si apre nella stessa scrittura.
func (b *bancoDU) r2Converti(w http.ResponseWriter, corpo m, prod string) m {
	cd := corpo["converti_da"].(m)
	vecchia := b.bozze[prod]
	if vecchia == nil || vecchia.Stato != "aperta" || !vecchia.Formato2 {
		bdKo(w, 422, "CORPO_NON_VALIDO", "nessuna bozza del formato di prima da convertire", m{"motivo": "conversione_ambigua"})
		return nil
	}
	if bdStr(cd["bozza_id"]) != vecchia.ID || bdNum(cd["versione_attesa"]) != vecchia.Versione {
		bdKo(w, 409, "BOZZA_VERSIONE", "La bozza da convertire è cambiata.", m{"motivo": "versione_diversa", "versione": vecchia.Versione})
		return nil
	}
	if bdStr(cd["motivo"]) == "" {
		bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "La conversione vuole il motivo.", m{"campo": "converti_da.motivo"})
		return nil
	}
	vecchia.Stato = "abbandonata"
	b.nBozze++
	bz := &bozzaBanco{ID: fmt.Sprintf("00000000-0000-4000-8000-0000000b%04d", b.nBozze), Stato: "aperta", Versione: 1, Creata: time.Now().UTC()}
	bz.Snapshot = bdSnapshotCanonico(m{"formato": 3, "prodotto_id": prod, "versione_bom_base": m{"conferma_id": nil, "numero": len(b.conferme[prod]), "contatore": 13}, "firma_base": strings.Repeat("3", 64),
		"nodi": []any{m{"chiave_bozza": "k-root", "componente_id": prod, "op": "riusa", "tipo": "finito", "origine": "registrato",
			"identita": m{"emittente": "cliente:" + bdCliente, "emittente_presunto": true, "codice": "ACM-1000", "rev": "C", "modo": "registrato"}}},
		"archi": []any{}, "decisioni": []any{}, "documenti_preassociati": []any{}})
	b.bozze[prod] = bz
	b.conti["conversioni"]++
	b.eventoServer(bz, "bozza_aperta", m{"partenza": "formato_2", "da_bozza": vecchia.ID})
	return m{"bozza_id": bz.ID, "formato": 3, "versione": 1, "nuova_versione": true, "riproposto": false, "eventi": 1, "impronta": bdSha(bdCanonico(bz.Snapshot)), "salvata_il": time.Now().UTC(),
		"snapshot": bz.Snapshot, "righe_cadute": []any{}}
}

func (b *bancoDU) r2CambiaSorgente(w http.ResponseWriter, corpo m, prod string) m {
	bz := b.bozzaAttesa(w, corpo, prod)
	if bz == nil {
		return nil
	}
	cs := corpo["cambia_sorgente"].(m)
	if bdStr(cs["motivo"]) == "" {
		bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "Il cambio della fonte vuole il motivo.", m{"campo": "cambia_sorgente.motivo"})
		return nil
	}
	sorg, _ := cs["sorgente"].(m)
	a := bdStr(sorg["allegato_id"])
	f := b.fileDi(a)
	if f == nil || f.Stato != "attivo" {
		bdKo(w, 422, "FILE_NON_ATTIVO", "lo STEP non è fra i file attivi", nil)
		return nil
	}
	reg := b.registro(a, f.Sha, b.versioneProposta)
	imp := bdImpronta(reg)
	b.registri[imp] = reg
	bz.Registro = imp
	bz.Versione++
	bz.Snapshot = bdSnapshotCanonico(b.statoDiPartenza(reg, imp))
	b.eventoServer(bz, "sorgente_cambiata", m{"allegato_id": a, "gruppo": reg["gruppo"].(m)["chiave"], "da_rivedere": []any{}})
	return m{"bozza_id": bz.ID, "formato": 3, "versione": bz.Versione, "nuova_versione": true, "riproposto": false, "eventi": 1, "impronta": bdSha(bdCanonico(bz.Snapshot)), "salvata_il": time.Now().UTC(),
		"snapshot": bz.Snapshot, "importazione": m{"importazione_id": "00000000-0000-4000-8000-0000000b1002", "impronta": imp, "impronta_contenuto": reg["impronta_contenuto"], "riusata": false,
			"fuori_per_cambio_fonte": []any{}, "caselle_riscritte": []any{}, "archi_riscritti": []any{}, "identita_aggiornate": []any{m{"chiave_bozza": "k-7"}}, "da_rivedere": []any{}, "decisioni_cadute": []any{}}}
}

func (b *bancoDU) r2Riallinea(w http.ResponseWriter, corpo m, prod string) m {
	bz := b.bozzaAttesa(w, corpo, prod)
	if bz == nil {
		return nil
	}
	rp := corpo["riallinea_proposta"].(m)
	reg := b.registri[bz.Registro]
	oggi := b.registro(reg["gruppo"].(m)["allegato_id"].(string), reg["gruppo"].(m)["sha256"].(string), b.versioneProposta)
	if bdStr(rp["impronta_contenuto"]) != oggi["impronta_contenuto"] {
		bdKo(w, 409, "ANTEPRIMA_SUPERATA", "La proposta è cambiata mentre guardavi.", m{"anteprima": "proposta_step"})
		return nil
	}
	if bdStr(rp["motivo"]) == "" {
		bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "L'aggiornamento vuole il motivo.", m{"campo": "riallinea_proposta.motivo"})
		return nil
	}
	imp := bdImpronta(oggi)
	b.registri[imp] = oggi
	bz.Registro = imp
	bz.Versione++
	snap := bz.Snapshot
	snap["sorgente"].(m)["importazione"] = imp
	for _, x := range snap["nodi"].([]any) {
		n := x.(m)
		if n["chiave_bozza"] == "k-11" && n["identita"].(m)["modo"] == "proposta" {
			n["identita"].(m)["codice"] = "ACM-1121"
		}
	}
	bz.Snapshot = bdSnapshotCanonico(snap)
	snap = bz.Snapshot
	b.eventoServer(bz, "proposta_riallineata", m{"importazione": imp, "algoritmo": "riancoraggio-1", "da_rivedere": []any{}, "identita_aggiornate": []any{"k-11"}})
	return m{"bozza_id": bz.ID, "formato": 3, "versione": bz.Versione, "nuova_versione": true, "riproposto": false, "eventi": 1, "impronta": bdSha(bdCanonico(snap)), "salvata_il": time.Now().UTC(),
		"snapshot": snap, "importazione": m{"importazione_id": "00000000-0000-4000-8000-0000000b1003", "impronta": imp, "impronta_contenuto": oggi["impronta_contenuto"], "riusata": false,
			"identita_aggiornate": []any{m{"chiave_bozza": "k-11", "prima": "ACM-1120", "dopo": "ACM-1121"}}, "da_rivedere": []any{}, "nuovi_nodi_proposti": []any{}}}
}

func (b *bancoDU) r3(w http.ResponseWriter, r *http.Request) {
	corpo, _, err := bdCorpo(r)
	if err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il corpo non è JSON", nil)
		return
	}
	for _, bz := range b.bozze {
		if bz.ID == r.PathValue("b") {
			if bz.Stato != "aperta" {
				// il reinvio dopo una risposta persa trova la bozza già chiusa (EM §8.4)
				bdKo(w, 409, "BOZZA_VERSIONE", "La bozza è già chiusa.", m{"motivo": "bozza_chiusa"})
				return
			}
			if bdNum(corpo["versione_attesa"]) != bz.Versione {
				bdKo(w, 409, "BOZZA_VERSIONE", "La bozza è cambiata altrove.", m{"motivo": "versione_diversa", "versione": bz.Versione})
				return
			}
			b.conti["R3"]++
			bz.Stato = "abbandonata"
			bdJSON(w, http.StatusOK, m{"stato": "abbandonata", "bozza_id": bz.ID})
			return
		}
	}
	bdKo(w, 422, "FUORI_RFQ", "la bozza non è di questa RFQ", nil)
}

// riepilogo: il riepilogo formato 3 (C3 §4.2 con le aggiunte di EM §9.4), calcolato dal banco con i soli blocchi e
// avvisi che servono ai flussi: CODICE_DA_ASSEGNARE, SEGNALATI_INCLUSI, NODI_PROPOSTI_NON_DECISI, VERIFICA_DOPO_COMMIT.
func (b *bancoDU) riepilogo(prod string, snap m, versione int, motore bool) m {
	bz := b.bozze[prod]
	var reg m
	if bz != nil {
		reg = b.registri[bz.Registro]
	}
	voci := map[string]m{}
	if reg != nil {
		for _, x := range reg["proposta"].(m)["nodi"].([]any) {
			v := x.(m)
			voci[bdStr(v["voce"])] = v
		}
	}
	codiceDi := map[string]string{}
	tipoDi := map[string]string{}
	identita := []any{}
	blocchi := []any{}
	presenti := map[string]bool{}
	segnalatiInclusi := []any{}
	nodi, _ := snap["nodi"].([]any)
	for _, x := range nodi {
		n := x.(m)
		k := bdStr(n["chiave_bozza"])
		id, _ := n["identita"].(m)
		codice := bdStr(id["codice"])
		codiceDi[k], tipoDi[k] = codice, bdStr(n["tipo"])
		var voce m
		if cs, ok := n["caselle"].([]any); ok && len(cs) > 0 {
			c := cs[0].(m)
			chiave := "n:" + bdStr(c["allegato_id"]) + ":" + bdStr(c["rif"])
			presenti[chiave] = true
			voce = voci[chiave]
			if voce != nil && len(voce["segnali"].([]any)) > 0 && bdStr(n["op"]) == "crea" {
				segnalatiInclusi = append(segnalatiInclusi, k)
			}
		}
		esito := map[string]string{"proposta": "accettata", "registrato": "registrata", "manuale": "manuale", "da_decidere": "da_assegnare", "decisione": "corretta"}[bdStr(id["modo"])]
		voceId := m{"chiave_bozza": k, "componente_id": n["componente_id"], "azione": n["op"], "origine": n["origine"], "codice": m{"prima": nil, "dopo": codice},
			"rev": m{"prima": nil, "dopo": bdStr(id["rev"])}, "tipo": m{"prima": nil, "dopo": n["tipo"]}, "collisioni": []any{}, "esito_identita": esito,
			"decisa":      m{"emittente": id["emittente"], "emittente_presunto": id["emittente_presunto"] == true, "namespace": id["namespace"], "codice": codice, "modo": id["modo"], "decisione": id["decisione"]},
			"riferimenti": []any{}, "alternative": m{"numero": 0}}
		if voce != nil && voce["identita_proposta"] != nil {
			ip := voce["identita_proposta"].(m)
			voceId["proposta"] = m{"letterale": ip["letterale"], "emittente": ip["emittente"], "namespace": ip["namespace"], "codice_proposto": voce["codice_proposto"]}
			voceId["riferimenti"] = voce["riferimenti"]
		}
		identita = append(identita, voceId)
		if bdStr(n["op"]) == "crea" && codice == "" {
			blocchi = append(blocchi, m{"codice": "CODICE_DA_ASSEGNARE", "chiave_bozza": k, "testo": "Il pezzo «" + bdStr(n["descrizione"]) + "» non ha un codice: assegnalo prima di confermare.",
				"rimedi": []any{"correggi il codice del pezzo (con il motivo, se è importato)", "non includerlo, se non è un articolo"}})
		}
	}
	nonInclusi := []any{}
	decisioni, _ := snap["decisioni"].([]any)
	for _, x := range decisioni {
		d := x.(m)
		if d["tipo"] != "non_includere" {
			continue
		}
		c := d["casella"].(m)
		chiave := "n:" + bdStr(c["allegato_id"]) + ":" + bdStr(c["rif"])
		presenti[chiave] = true
		nome := ""
		if v := voci[chiave]; v != nil {
			nome = bdStr(v["grezzi"].(m)["nome"])
		}
		nonInclusi = append(nonInclusi, m{"id": d["id"], "casella": c, "nome_grezzo": nome, "segnale_visto": d["segnale_visto"], "segnale_registro": d["segnale_visto"], "motivo_codice": d["motivo_codice"],
			"motivo": d["motivo"], "azione_id": d["azione_id"], "autore_id": duUUID(77).String(),
			"effetti": m{"righe_da_chiudere": []any{m{"proposta_id": "00000000-0000-4000-8000-0000000c0031", "allegato_id": bdA1}}, "archi_da_chiudere": []any{m{"allegato_id": bdA1, "padre_chiave": "#7", "figlio_chiave": "#3x", "padre_nella_bozza": true}},
				"figli": []any{}, "occorrenze_toccate": []any{bdA1}, "prodotti_con_lo_stesso_step": []any{}, "fabbisogni_tolti_dopo_conferma": []any{m{"tipo_documento": "disegno_2d", "bloccante": true}}}})
	}
	avvisi := []any{m{"codice": "VERIFICA_DOPO_COMMIT", "testo": "BOM.Verificata si calcola sulla struttura confermata, dopo il commit"}}
	// le regole minime del banco per D1, mono_pezzo, svincoli, esclusi con documenti ed equivalenti K4 (EM §8.1, §8.2): non
	// sono quelle del server, servono a far vedere nella pagina i rimedi e gli avvisi
	figli := map[string]int{}
	if as, ok := snap["archi"].([]any); ok {
		for _, x := range as {
			figli[bdStr(x.(m)["padre"])]++
		}
	}
	d1 := map[string]string{}
	monoPezzo := false
	scollegati := map[string]bool{}
	svincoli := []any{}
	for _, x := range decisioni {
		d := x.(m)
		switch bdStr(d["tipo"]) {
		case "d1_a_particolare", "d1_resta_sottoassieme", "d1_a_sottoassieme":
			d1[bdStr(d["chiave_bozza"])] = bdStr(d["tipo"])
		case "mono_pezzo":
			monoPezzo = true
		case "scollega_documento":
			pid := bdStr(d["proposta_id"])
			scollegati[pid] = true
			esito := "applicabile"
			for _, f := range b.file {
				if f.Proposta == pid && f.Documento != nil {
					esito = "svincolo_su_documento"
					blocchi = append(blocchi, m{"codice": "PREASSOCIAZIONE_NON_APPLICABILE", "motivo": "svincolo_su_documento", "testo": "«" + f.Nome + "» è un documento confermato: non si scollega da qui (serve una revisione, non in V1)."})
				}
			}
			svincoli = append(svincoli, m{"proposta_id": pid, "componente_id": d["componente_id"], "esito": esito})
		}
	}
	for _, x := range nodi {
		n := x.(m)
		k := bdStr(n["chiave_bozza"])
		if bdStr(n["componente_id"]) == prod {
			if figli[k] == 0 && !monoPezzo {
				avvisi = append(avvisi, m{"codice": "PRODOTTO_SENZA_FIGLI", "chiave_bozza": k, "testo": "Il prodotto non ha figli nella bozza: se è un pezzo unico, dichiara mono_pezzo con il motivo."})
			}
			continue
		}
		var voce m
		if cs, ok := n["caselle"].([]any); ok && len(cs) > 0 {
			c := cs[0].(m)
			voce = voci["n:"+bdStr(c["allegato_id"])+":"+bdStr(c["rif"])]
		}
		tipo, op := bdStr(n["tipo"]), bdStr(n["op"])
		if op == "esclude" {
			n2 := 0
			for _, f := range b.file {
				if f.Componente != "" && f.Componente == bdStr(n["componente_id"]) && f.Documento == nil && !scollegati[f.Proposta] {
					n2++
				}
			}
			if n2 > 0 {
				avvisi = append(avvisi, m{"codice": "ESCLUSO_CON_DOCUMENTI", "chiave_bozza": k, "testo": fmt.Sprintf("%s esce dalla BOM con %d associazioni di file registrate: restano storiche.", codiceDi[k], n2)})
			}
			continue
		}
		if tipo == "sottoassieme" && figli[k] == 0 && voce != nil && voce["foglia"] == false && d1[k] != "d1_a_particolare" && d1[k] != "d1_resta_sottoassieme" {
			blocchi = append(blocchi, m{"codice": "D1_ULTIMO_FIGLIO", "chiave_bozza": k, "testo": codiceDi[k] + " è un assieme rimasto senza figli: diventa un particolare, oppure resta assieme.",
				"rimedi": []any{"Diventa particolare", "Lascia assieme"}})
		}
		if (tipo == "sciolto" || tipo == "commerciale") && figli[k] > 0 && d1[k] != "d1_a_sottoassieme" {
			blocchi = append(blocchi, m{"codice": "D1_PARTICOLARE_CON_FIGLI", "chiave_bozza": k, "testo": codiceDi[k] + " è un particolare con figli: diventa un assieme.", "rimedi": []any{"Diventa assieme"}})
		}
		if voce != nil {
			id, _ := n["identita"].(m)
			for _, rr := range voce["riferimenti"].([]any) {
				if bdStr(rr.(m)["tipo"]) == "equivalente_k4" && bdStr(id["modo"]) != "decisione" {
					avvisi = append(avvisi, m{"codice": "EQUIVALENTE_NON_ADOTTATO", "chiave_bozza": k, "testo": codiceDi[k] + " va nella working con un codice diverso dal P/N equivalente del fornitore, senza una decisione che lo adotti o lo rifiuti."})
				}
			}
		}
	}
	if len(segnalatiInclusi) > 0 {
		avvisi = append(avvisi, m{"codice": "SEGNALATI_INCLUSI", "testo": fmt.Sprintf("%d nodi segnalati come possibili elementi CAD stanno per diventare componenti.", len(segnalatiInclusi)), "chiavi": segnalatiInclusi})
	}
	mancanti := 0
	for k, v := range voci {
		if sm, _ := v["stato_motore"].(m); !presenti[k] && sm["tolto"] != true {
			mancanti++
		}
	}
	if mancanti > 0 {
		avvisi = append(avvisi, m{"codice": "NODI_PROPOSTI_NON_DECISI", "testo": fmt.Sprintf("%d voci della proposta non sono nella bozza e non hanno una decisione: le loro righe restano aperte.", mancanti)})
	}
	archi := []any{}
	archiSnap, _ := snap["archi"].([]any)
	for _, x := range archiSnap {
		a := x.(m)
		p, f := bdStr(a["padre"]), bdStr(a["figlio"])
		archi = append(archi, m{"padre": m{"chiave_bozza": p, "codice": codiceDi[p], "tipo": tipoDi[p]}, "figlio": m{"chiave_bozza": f, "codice": codiceDi[f], "tipo": tipoDi[f]},
			"qta_step": a["qta"], "qta_precedente": nil, "qta_voluta": a["qta"], "azione": "aggiunto", "origine": a["origine"], "decisioni": []any{}, "altri_padri": []any{}})
	}
	pre := []any{}
	preSnap, _ := snap["documenti_preassociati"].([]any)
	for _, x := range preSnap {
		d := x.(m)
		esito := "in_attesa_codice"
		for _, f := range b.file {
			if f.Proposta == bdStr(d["proposta_id"]) {
				switch {
				case f.Stato != "attivo":
					esito = "orfana"
				case codiceDi[bdStr(d["chiave_bozza"])] == "":
					esito = "in_attesa_codice"
				case f.Codice == codiceDi[bdStr(d["chiave_bozza"])]:
					esito = "applicabile"
				default:
					esito = "in_conflitto"
				}
				pre = append(pre, m{"proposta_id": d["proposta_id"], "chiave_bozza": d["chiave_bozza"], "nome_file": f.Nome, "codice_proposta": f.Codice, "rev_proposta": f.Rev, "esito": esito})
			}
		}
		if esito == "in_conflitto" {
			blocchi = append(blocchi, m{"codice": "PREASSOCIAZIONE_NON_APPLICABILE", "chiave_bozza": d["chiave_bozza"], "testo": "Il codice del file non è quello del pezzo: togli la preassociazione o correggi.", "motivo": "codice_diverso"})
		}
	}
	relazioni := []any{}
	if reg != nil {
		for _, x := range reg["proposta"].(m)["relazioni"].([]any) {
			rr := x.(m)
			relazioni = append(relazioni, m{"tipo": rr["tipo"], "stato": rr["stato"], "lettura_sorgente": rr["lettura_sorgente"], "letture_bersaglio": rr["letture_bersaglio"], "evidenze": rr["evidenze"], "ratifica": "non_ratificata"})
		}
	}
	o := m{"formato": 3, "prodotto_id": prod, "versione_corrente": bdVersioneCorrente(b.conferme[prod]), "base": m{"stato": "valida", "motivi": []any{}}, "rif_scaduti": []any{},
		"identita": identita, "archi": archi, "da_ricollocare": []any{}, "fuori": []any{}, "non_inclusi": nonInclusi, "relazioni": relazioni, "svincoli": svincoli,
		"registro_decisioni": []any{}, "documenti_preassociati": pre, "righe_step": m{"decise_da_questa_conferma": len(nodi), "restano_aperte": mancanti},
		"criterio_previsto": m{"criterio": "nessuno", "motivo": "il prodotto non ha uno STEP autorizzato: dopo l'autorizzazione serve una conferma nuova"},
		"blocchi":           blocchi, "avvisi": avvisi, "avvisi_motore": []any{}, "avvisi_motore_calcolati": false, "confermabile": len(blocchi) == 0}
	if bz != nil && bz.Registro != "" {
		o["importazione"] = m{"impronta": bz.Registro, "impronta_contenuto": reg["impronta_contenuto"]}
	}
	// il segno di PE-23, solo con l'interruttore uguale-alla-base (il server di oggi non lo dà)
	if u, ok := b.ugualeAllaBase(prod, snap); ok {
		o["uguale_alla_base"] = u
	}
	if motore {
		if b.modo("motore") == "ko" {
			o["avvisi_motore_errore"] = m{"codice": "CALCOLO_OCCUPATO", "testo": "Il motore A sta già facendo altri calcoli."}
		} else {
			o["avvisi_motore_calcolati"] = true
		}
	}
	// la firma sul JSON canonico dello snapshot (EM §8.3, §3.10): lo stesso contenuto in un altro ordine ha la stessa firma
	o["firma_anteprima"] = bdSha(fmt.Sprintf("anteprima-3|%d|%s", versione, bdCanonico(bdFormaCanonica(snap, ""))))
	return o
}

func (b *bancoDU) r4(w http.ResponseWriter, r *http.Request) {
	corpo, _, err := bdCorpo(r)
	if err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il corpo non è JSON", nil)
		return
	}
	prod := bdStr(corpo["prodotto_id"])
	if prod != bdP1 && prod != bdP2 {
		bdKo(w, 422, "FUORI_RFQ", "il prodotto non è di questa RFQ", nil)
		return
	}
	snap := m{}
	for k, v := range corpo {
		if k != "bozza_id" && k != "bozza_versione" && k != "chiave_idempotenza" {
			snap[k] = v
		}
	}
	b.conti["R4"]++
	bdJSON(w, http.StatusOK, b.riepilogo(prod, snap, bdNum(corpo["bozza_versione"]), r.URL.Query().Get("avvisi_motore") == "1"))
}

// r5: la conferma (EM §9.5). L'intento si cerca prima di tutto; la risposta registrata torna con «riproposto».
func (b *bancoDU) r5(w http.ResponseWriter, r *http.Request) {
	corpo, _, err := bdCorpo(r)
	if err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il corpo non è JSON", nil)
		return
	}
	bozza, _ := corpo["bozza"].(m)
	chiave := bdStr(bozza["chiave_idempotenza"])
	if chiave == "" || bdStr(bozza["bozza_id"]) == "" {
		bdKo(w, 422, "CORPO_NON_VALIDO", "servono chiave_idempotenza e bozza_id", m{"motivo": "bozza_obbligatoria"})
		return
	}
	b.chiaviR5 = append(b.chiaviR5, chiave)
	senzaVersione := m{}
	for k, v := range bozza {
		if k != "bozza_versione" && k != "chiave_idempotenza" {
			senzaVersione[k] = v
		}
	}
	// impronta_corpo sulla forma canonica (EM §8.4, §3.10): lo stesso contenuto in un altro ordine è lo stesso corpo
	impronta := bdSha(bdCanonico(bdFormaCanonica(m{"firma_anteprima": corpo["firma_anteprima"], "bozza": senzaVersione}, "")))
	if it, ok := b.intenti[chiave]; ok {
		if it.Impronta != impronta || it.Gesto != "conferma_bom_v3" {
			bdKo(w, 409, "CHIAVE_RIUSATA", "Questo tentativo ha già un esito diverso.", nil)
			return
		}
		rr := m{}
		for k, v := range it.Risposta {
			rr[k] = v
		}
		rr["riproposto"] = true
		bdJSON(w, http.StatusOK, rr)
		return
	}
	prod := bdStr(bozza["prodotto_id"])
	bz := b.bozze[prod]
	switch b.modo("R5") {
	case "409_base":
		bdKo(w, 409, "BASE_SUPERATA", "La distinta è cambiata dopo la tua lettura: la bozza resta, rileggi la base e riconferma.", m{"motivi": []any{"conferma_cambiata"}, "versione_corrente": bdVersioneCorrente(b.conferme[prod])})
		return
	case "422":
		bdKo(w, 422, "BLOCCHI", "La BOM non si conferma: 1 blocco.", m{"blocchi": []any{m{"codice": "D1_ULTIMO_FIGLIO", "chiave_bozza": "k-7", "testo": "ACM-1100 resta senza figli.",
			"rimedi": []any{"Diventa particolare", "Lascia assieme"}}}})
		return
	case "controllo_finale":
		bdJSON(w, http.StatusInternalServerError, m{"esito": "non_registrato", "codice": "CONTROLLO_FINALE", "testo": "La conferma non è registrata per un errore interno del controllo finale: non riprovare, segnala."})
		return
	case "500_prima":
		bdJSON(w, http.StatusInternalServerError, m{"esito": "da_verificare", "chiave_idempotenza": chiave, "testo": "Non so se la conferma è stata registrata: verifico con la stessa chiave."})
		return
	}
	if bz == nil || bz.Stato != "aperta" || bdStr(bozza["bozza_id"]) != bz.ID || bdNum(bozza["bozza_versione"]) != bz.Versione {
		bdKo(w, 409, "BOZZA_VERSIONE", "La bozza salvata non è quella che stai confermando.", m{"motivo": "versione_diversa"})
		return
	}
	snap := m{}
	for k, v := range bozza {
		if k != "bozza_id" && k != "bozza_versione" && k != "chiave_idempotenza" {
			snap[k] = v
		}
	}
	rip := b.riepilogo(prod, snap, bz.Versione, false)
	if rip["firma_anteprima"] != corpo["firma_anteprima"] {
		bdKo(w, 409, "ANTEPRIMA_SUPERATA", "Il riepilogo è cambiato: rileggilo e conferma di nuovo.", m{"anteprima": "riepilogo_bom", "riepilogo": rip})
		return
	}
	// PE-23: una bozza uguale alla conferma di base formato 3 non si conferma (con l'interruttore uguale-alla-base)
	if u, ok := b.ugualeAllaBase(prod, snap); ok && u {
		bdKo(w, 422, "NESSUNA_MODIFICA", "La bozza è uguale alla conferma di base: niente da confermare.", nil)
		return
	}
	if !rip["confermabile"].(bool) {
		bdKo(w, 422, "BLOCCHI", fmt.Sprintf("La BOM non si conferma: %d blocchi.", len(rip["blocchi"].([]any))), m{"blocchi": rip["blocchi"]})
		return
	}
	numero := len(b.conferme[prod]) + 1
	cid := fmt.Sprintf("00000000-0000-4000-8000-0000000f%04d", numero)
	mappa := m{}
	accettate := []any{}
	for _, x := range snap["nodi"].([]any) {
		n := x.(m)
		k := bdStr(n["chiave_bozza"])
		mappa[k] = fmt.Sprintf("00000000-0000-4000-8000-%012d", 5000+len(mappa))
		if n["identita"].(m)["modo"] == "proposta" {
			accettate = append(accettate, k)
		}
	}
	nonInc := []any{}
	monoPezzo := false
	decs, _ := snap["decisioni"].([]any)
	for _, x := range decs {
		if x.(m)["tipo"] == "non_includere" {
			nonInc = append(nonInc, x.(m)["id"])
		}
		if x.(m)["tipo"] == "mono_pezzo" {
			monoPezzo = true
		}
	}
	risposta := m{"esito": "confermata", "riproposto": false, "chiave_idempotenza": chiave, "conferma_id": cid, "numero_conferma": numero, "prodotto_id": prod, "formato": 3,
		"firma_struttura": strings.Repeat("3", 64), "contatore": 13, "mappa": mappa, "identita_accettate": accettate, "identita_corrette": []any{}, "non_inclusi_applicati": nonInc,
		"associazioni_applicate": snap["documenti_preassociati"], "svincoli_applicati": rip["svincoli"], "relazioni_non_ratificate": rip["relazioni"],
		"verifica_motore_a": m{"verificata": false, "criterio": "nessuno", "motivi": []any{"verifica_non_calcolata_v1"}, "al_momento_della_conferma": true}, "f8_stato": "in_attesa",
		"testo": fmt.Sprintf("Conferma n. %d di ACM-1000.", numero)}
	b.conti["R5"]++
	b.intenti[chiave] = intentoBanco{Gesto: "conferma_bom_v3", Impronta: impronta, Risposta: risposta, Creato: time.Now().UTC()}
	b.conferme[prod] = append(b.conferme[prod], m{"conferma_id": cid, "numero": numero, "formato": 3, "senza_riepilogo_v2": false, "ultima": true, "valida": true,
		"confermato_da": duUUID(77).String(), "confermato_da_nome": "AD", "confermato_il": time.Now().UTC(), "firma_struttura": strings.Repeat("3", 64), "criterio_verifica": "nessuno", "mono_pezzo": monoPezzo,
		"origine": m{"rotta": "motore-a/bom/conferma"}, "bozza_id": bz.ID, "fonte": snap["sorgente"], "importazione_id": "00000000-0000-4000-8000-0000000b1001"})
	for _, c := range b.conferme[prod][:numero-1] {
		c["ultima"] = false
	}
	bz.Stato = "confermata"
	bdJSON(w, http.StatusOK, risposta)
}

func (b *bancoDU) r6(w http.ResponseWriter, r *http.Request) {
	k := r.PathValue("k")
	b.conti["R6_letture"]++
	b.chiaviR6 = append(b.chiaviR6, k)
	if b.modo("R6") == "in_corso" {
		bdJSON(w, http.StatusOK, m{"stato": "in_corso"})
		return
	}
	it, ok := b.intenti[k]
	if !ok {
		bdJSON(w, http.StatusOK, m{"stato": "non_registrato"})
		return
	}
	o := m{"stato": "registrato", "gesto": it.Gesto, "creato_il": it.Creato, "esito": it.Risposta}
	if it.Gesto == "conferma_bom_v3" {
		o["formato"] = 3
		o["f8_stato"] = "eseguito"
	} else if f := b.fileDi(it.Risorsa); f != nil {
		o["stati_correnti"] = m{"rimozione": f.Rimozione, "conservazione": f.Conservazione}
	}
	bdJSON(w, http.StatusOK, o)
}

func (b *bancoDU) r7(w http.ResponseWriter, r *http.Request) {
	prod := r.URL.Query().Get("prodotto")
	conf := b.conferme[prod]
	if conf == nil {
		conf = []m{}
	}
	bdJSON(w, http.StatusOK, m{"prodotto_id": prod, "conferme": conf, "attestazioni": []any{}, "fonte": []any{}})
}

func (b *bancoDU) r11(w http.ResponseWriter, _ *http.Request) {
	b.conti["R11"]++
	b.f8Errore = false
	bdJSON(w, http.StatusOK, m{"f8_stato": "eseguito"})
}

// ---------------------------------------------------------------- E1′ e la Sorgente (S1-S7)

func (b *bancoDU) e1(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept"), "application/json") {
		http.Error(w, "il banco serve solo il ramo JSON", http.StatusBadRequest)
		return
	}
	if b.modo("E1") == "appeso" {
		// la prova del tempo d'inattività: il banco ha aspettato (ritardo) più del tempo d'inattività del ponte, la pagina
		// ha già lasciato perdere; niente si registra
		bdKo(w, 503, "NON_CONFIGURATO", "caricamento di prova lasciato cadere dopo l'attesa", nil)
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "multipart non valido", nil)
		return
	}
	origine := r.FormValue("origine")
	if origine == "" {
		origine = "portale"
	}
	if origine != "portale" && origine != "interna" {
		bdKo(w, 422, "CORPO_NON_VALIDO", "origine sconosciuta", m{"campo": "origine"})
		return
	}
	b.conti["E1"]++
	esiti := []any{}
	caricati, rifiutati := 0, 0
	var fh []*multipart.FileHeader
	if r.MultipartForm != nil {
		fh = r.MultipartForm.File["file"]
	}
	for _, h := range fh {
		e := m{"nome_file": h.Filename, "origine": origine, "motivo": nil, "ricaricato_dopo_scarto": false}
		switch {
		case strings.HasSuffix(strings.ToLower(h.Filename), ".exe"):
			e["esito"] = "rifiutato"
			e["motivo"] = m{"codice": "TIPO_FILE", "testo": "tipo di file non ammesso"}
			rifiutati++
		default:
			var gia *fileBanco
			dopoScarto := false
			for _, f := range b.file {
				if f.Nome == h.Filename && f.Contenitore == "" {
					if f.Stato == "scartato" {
						dopoScarto = true
						continue
					}
					gia = f
				}
			}
			if gia != nil {
				e["esito"], e["allegato_id"], e["proposta_id"], e["sha256"], e["stato_file"], e["analisi"] = "gia_presente", gia.ID, gia.Proposta, gia.Sha, gia.Stato, gia.Analisi
				break
			}
			id := fmt.Sprintf("00000000-0000-4000-8000-0000000a%04x", 0x100+len(b.file))
			est := strings.TrimPrefix(strings.ToLower(h.Filename[strings.LastIndex(h.Filename, ".")+1:]), ".")
			nf := &fileBanco{ID: id, Proposta: bdProposta(id), Nome: h.Filename, Est: est, Sha: bdSha(h.Filename), Origine: origine, Canale: "caricamento", Stato: "attivo", Analisi: "in_analisi"}
			b.file = append(b.file, nf)
			e["esito"], e["allegato_id"], e["proposta_id"], e["sha256"], e["stato_file"], e["analisi"], e["ricaricato_dopo_scarto"] = "caricato", id, nf.Proposta, nf.Sha, "attivo", "in_analisi", dopoScarto
			caricati++
		}
		esiti = append(esiti, e)
	}
	if caricati == 0 && rifiutati > 0 {
		bdKo(w, 422, "NESSUN_FILE_CARICATO", "Nessun file è stato caricato.", m{"file": esiti})
		return
	}
	esito := "caricati"
	if rifiutati > 0 {
		esito = "parziale"
	}
	bdJSON(w, http.StatusOK, m{"esito": esito, "origine": origine, "file": esiti, "testo": fmt.Sprintf("%d file caricati, %d rifiutati.", caricati, rifiutati)})
}

// dipendenze: le dipendenze di un'occorrenza (EM §10.3), con le regole minime del banco: la fonte della bozza e le
// preassociazioni bloccano (D-6), l'unica copia chiede la conferma (D-14), lo zip porta le dipendenze delle voci.
func (b *bancoDU) dipendenze(f *fileBanco) []any {
	out := []any{}
	bz := b.bozze[bdP1]
	usato := func(id string) (bool, string) {
		if bz == nil || bz.Stato != "aperta" {
			return false, ""
		}
		if s, ok := bz.Snapshot["sorgente"].(m); ok && bdStr(s["allegato_id"]) == id {
			return true, "fonte della bozza"
		}
		if pre, ok := bz.Snapshot["documenti_preassociati"].([]any); ok {
			for _, x := range pre {
				if fb := b.fileDi(id); fb != nil && bdStr(x.(m)["proposta_id"]) == fb.Proposta {
					return true, "preassociazione al pezzo " + bdStr(x.(m)["chiave_bozza"])
				}
			}
		}
		return false, ""
	}
	controlla := func(x *fileBanco) {
		if ok, come := usato(x.ID); ok {
			out = append(out, m{"allegato_id": x.ID, "codice": "D-6", "esito": "blocca", "testo": "È usato dalla bozza di ACM-1000 (" + come + "): cambia la bozza e salvala, poi riprova."})
		}
		if x.Origine == "interna" {
			out = append(out, m{"allegato_id": x.ID, "codice": "D-14", "esito": "risoluzione", "risoluzione": "conferma_unica_copia", "testo": "È l'unica copia: il file era stato caricato a mano. Scartandolo non ne resta nessuna copia: per riaverlo dovrai ricaricarlo."})
		}
		if x.Componente != "" {
			cod := map[string]string{bdP1: "ACM-1000", bdC40: "ACM-1400"}[x.Componente]
			out = append(out, m{"allegato_id": x.ID, "codice": "D-7", "esito": "risoluzione", "risoluzione": "scollega", "testo": "È associato a " + cod + ": scartandolo, l'associazione viene tolta."})
		}
	}
	controlla(f)
	for _, v := range b.file {
		if v.Contenitore == f.ID && v.Stato != "scartato" {
			controlla(v)
		}
	}
	return out
}

func (b *bancoDU) voci(f *fileBanco) []*fileBanco {
	var out []*fileBanco
	for _, v := range b.file {
		if v.Contenitore == f.ID && v.Stato != "scartato" {
			out = append(out, v)
		}
	}
	return out
}

func (b *bancoDU) fileJSON(f *fileBanco) m {
	sc := m{"disponibile": true, "motivo": ""}
	if f.Stato == "scartato" {
		sc = m{"disponibile": false, "motivo": "il file è già scartato"}
	}
	af := m{"disponibile": f.Stato == "attivo", "motivo": map[bool]string{true: "", false: "solo un file attivo va in Altri file"}[f.Stato == "attivo"]}
	ri := m{"disponibile": f.Stato == "altri_file", "motivo": map[bool]string{true: "", false: "solo un file in Altri file si riattiva"}[f.Stato == "altri_file"]}
	o := m{"allegato_id": f.ID, "proposta_id": f.Proposta, "nome_file": f.Nome, "estensione": f.Est, "sha256": f.Sha, "bytes": 1024, "origine": f.Origine, "canale": f.Canale,
		"messaggio_id": "00000000-0000-4000-8000-0000000e0001", "ricevuto_il": "2026-10-15T08:00:00Z", "contenitore_id": nil, "path_interno": "", "voci": []any{}, "stato": f.Stato, "versione": f.Versione,
		"scartato_legacy": false, "proposta": m{"stato": "aperta", "tipo_proposto": "documento", "codice": f.Codice, "rev": f.Rev, "componente_id": nil}, "documento": nil,
		"analisi": m{"stato": f.Analisi, "motivo": ""}, "byte": m{"staging": "presente", "conservazione": "assente", "nas": "assente"}, "rimozione": f.Rimozione, "conservazione": f.Conservazione,
		"azioni": m{"scarta": sc, "altri_file": af, "riattiva": ri, "contenuto": m{"disponibile": f.Stato != "scartato", "motivo": "", "richiede": []any{}}}}
	if f.Componente != "" {
		o["proposta"].(m)["componente_id"] = f.Componente
	}
	if f.Contenitore != "" {
		o["contenitore_id"] = f.Contenitore
		o["path_interno"] = "pacchetto.zip/" + f.Nome
	}
	if f.Stato == "altri_file" {
		o["byte"] = m{"staging": "presente", "conservazione": "presente", "nas": "assente"}
	}
	if f.Scarto != nil {
		o["scarto"] = f.Scarto
	}
	// chi ha messo il file in Altri file, quando e perché (MAT FL-08): la forma proposta nel resoconto della corsia (PE-06)
	if f.AltriFile != nil && f.Stato == "altri_file" {
		o["altri_file"] = f.AltriFile
	}
	if f.Documento != nil {
		o["documento"] = f.Documento
	}
	return o
}

func (b *bancoDU) s1(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	b.conti["S1_letture"]++
	// il banco fa avanzare i lavori a ogni lettura: un file caricato passa da «in analisi» a «letto» alla seconda
	// lettura, una rimozione «in attesa» si completa alla seconda lettura dopo lo scarto
	for _, f := range b.file {
		f.letture++
		if f.Analisi == "in_analisi" && f.letture >= 2 {
			f.Analisi = "letto"
		}
		if f.Rimozione != nil && f.Rimozione["stato"] == "in_attesa" && f.letture >= 2 {
			f.Rimozione = m{"stato": "eseguita", "motivo": "rimosso", "testo": "File scartato ed eliminato dallo staging.", "tentativi": 1}
		}
	}
	file := []any{}
	contatori := m{"attivi": 0, "altri_file": 0, "scartati": 0, "in_analisi": 0}
	step := []any{}
	for _, f := range b.file {
		switch f.Stato {
		case "attivo":
			contatori["attivi"] = contatori["attivi"].(int) + 1
		case "altri_file":
			contatori["altri_file"] = contatori["altri_file"].(int) + 1
		case "scartato":
			contatori["scartati"] = contatori["scartati"].(int) + 1
		}
		if f.Analisi == "in_analisi" {
			contatori["in_analisi"] = contatori["in_analisi"].(int) + 1
		}
		if f.Stato == "scartato" && q.Get("scartati") != "1" {
			continue
		}
		file = append(file, b.fileJSON(f))
		if (f.Est == "stp" || f.Est == "step") && f.Stato == "attivo" {
			step = append(step, m{"allegato_id": f.ID, "nome_file": f.Nome, "analisi": m{"stato": f.Analisi}})
		}
	}
	var firma strings.Builder
	for _, f := range b.file {
		fmt.Fprintf(&firma, "%s|%s|%d|%s|%v|%s;", f.ID, f.Stato, f.Versione, f.Analisi, f.Rimozione, f.Componente)
	}
	o := m{"thread_id": bdThread, "firma": bdSha(firma.String()), "configurazione": m{"conservazione": true}, "contatori": contatori,
		"file": file, "step": step, "dettaglio": nil}
	if id := q.Get("file"); id != "" {
		f := b.fileDi(id)
		if f == nil {
			bdKo(w, 422, "FUORI_RFQ", "il file non è di questa RFQ", nil)
			return
		}
		dip := b.dipendenze(f)
		det := m{"allegato_id": f.ID, "dipendenze": dip, "impronta_dipendenze": bdSha(bdCanonico(dip)), "eventi": []any{}}
		if vv := b.voci(f); len(vv) > 0 {
			ids := []any{}
			elenco := []any{}
			for _, v := range vv {
				ids = append(ids, v.ID)
				elenco = append(elenco, m{"allegato_id": v.ID, "nome_file": v.Nome, "stato": v.Stato})
			}
			det["n_voci"] = len(vv)
			det["impronta_voci"] = bdSha(bdCanonico(ids))
			det["voci"] = elenco
		}
		o["dettaglio"] = det
	}
	bdJSON(w, http.StatusOK, o)
}

// gestoFile: le regole comuni di S2, S3, S4 (EM §10.9): intento, versione attesa, dipendenze e perimetro delle voci.
func (b *bancoDU) gestoFile(w http.ResponseWriter, r *http.Request, gesto string) (*fileBanco, m, string, bool) {
	f := b.fileDi(r.PathValue("a"))
	if f == nil {
		bdKo(w, 422, "FUORI_RFQ", "il file non è di questa RFQ", nil)
		return nil, nil, "", false
	}
	corpo, _, err := bdCorpo(r)
	if err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il corpo non è JSON", nil)
		return nil, nil, "", false
	}
	for k := range corpo {
		if strings.Contains(strings.ToLower(k), "percorso") || strings.Contains(strings.ToLower(k), "path") {
			bdKo(w, 422, "CORPO_NON_VALIDO", "campo sconosciuto: "+k, m{"campo": k})
			return nil, nil, "", false
		}
	}
	b.corpiFile = append(b.corpiFile, m{"gesto": gesto, "allegato": f.ID, "corpo": corpo})
	chiave := bdStr(corpo["chiave_idempotenza"])
	senza := m{}
	for k, v := range corpo {
		if k != "chiave_idempotenza" {
			senza[k] = v
		}
	}
	impronta := bdSha(bdCanonico(m{"gesto": gesto, "thread": bdThread, "allegato": f.ID, "corpo": senza}))
	if it, ok := b.intenti[chiave]; ok {
		if it.Impronta != impronta {
			bdKo(w, 409, "CHIAVE_RIUSATA", "la chiave ha già un esito diverso", nil)
			return nil, nil, "", false
		}
		rr := m{}
		for k, v := range it.Risposta {
			rr[k] = v
		}
		rr["riproposto"] = true
		bdJSON(w, http.StatusOK, rr)
		return nil, nil, "", false
	}
	if bdNum(corpo["versione_attesa"]) != f.Versione {
		bdKo(w, 409, "FILE_VERSIONE", "Lo stato del file è cambiato.", m{"stato": f.Stato, "versione": f.Versione})
		return nil, nil, "", false
	}
	return f, corpo, impronta, true
}

func (b *bancoDU) controllaDipendenze(w http.ResponseWriter, f *fileBanco, corpo m, gesto string) bool {
	dip := b.dipendenze(f)
	if gesto != "riattiva_file" && bdStr(corpo["impronta_dipendenze"]) != bdSha(bdCanonico(dip)) {
		bdKo(w, 409, "ANTEPRIMA_SUPERATA", "Le dipendenze del file sono cambiate.", m{"anteprima": "dipendenze_file", "dipendenze": dip})
		return false
	}
	if vv := b.voci(f); len(vv) > 0 {
		ids := []any{}
		for _, v := range vv {
			ids = append(ids, v.ID)
		}
		if bdNum(corpo["n_voci"]) != len(vv) || bdStr(corpo["impronta_voci"]) != bdSha(bdCanonico(ids)) {
			bdKo(w, 409, "ANTEPRIMA_SUPERATA", "Il perimetro delle voci dell'archivio è cambiato.", m{"anteprima": "voci_archivio"})
			return false
		}
	}
	var blocchi, mancano []any
	for _, x := range dip {
		d := x.(m)
		switch {
		case d["esito"] == "blocca":
			blocchi = append(blocchi, d)
		case d["esito"] == "risoluzione" && d["risoluzione"] == "conferma_unica_copia" && corpo["conferma_unica_copia"] != true && gesto == "scarta_file":
			mancano = append(mancano, d)
		case d["esito"] == "risoluzione" && d["risoluzione"] == "scollega" && corpo["scollega"] != true:
			mancano = append(mancano, d)
		}
	}
	if len(blocchi) > 0 {
		bdKo(w, 422, "DIPENDENZE_ATTIVE", "Il file ha dipendenze attive.", m{"dipendenze": blocchi})
		return false
	}
	if len(mancano) > 0 {
		bdKo(w, 422, "RISOLUZIONE_RICHIESTA", "Manca una risoluzione dichiarata.", m{"occorrenze": mancano})
		return false
	}
	return true
}

func (b *bancoDU) registraFile(chiave, impronta, gesto, risorsa string, risposta m) {
	b.intenti[chiave] = intentoBanco{Gesto: gesto, Impronta: impronta, Risorsa: risorsa, Risposta: risposta, Creato: time.Now().UTC()}
}

func (b *bancoDU) s2(w http.ResponseWriter, r *http.Request) {
	f, corpo, impronta, ok := b.gestoFile(w, r, "scarta_file")
	if !ok {
		return
	}
	if f.Stato == "scartato" {
		bdKo(w, 409, "FILE_VERSIONE", "Il file è già scartato.", m{"stato": f.Stato, "versione": f.Versione})
		return
	}
	if bdStr(corpo["motivo"]) == "" {
		bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "«Scarta» vuole il motivo.", m{"campo": "motivo"})
		return
	}
	if !b.controllaDipendenze(w, f, corpo, "scarta_file") {
		return
	}
	b.conti["S2"]++
	rimozioni := []any{}
	for _, x := range append([]*fileBanco{f}, b.voci(f)...) {
		x.Stato, x.Versione = "scartato", x.Versione+1
		x.Rimozione = m{"stato": "in_attesa", "motivo": "da_eseguire", "testo": "Scarto registrato; rimozione dallo staging ancora da completare.", "tentativi": 0}
		x.Scarto = m{"autore": "AD", "quando": time.Now().UTC(), "motivo": corpo["motivo"]}
		x.letture = 0
		rimozioni = append(rimozioni, m{"allegato_id": x.ID, "stato": "in_attesa", "motivo": "da_eseguire", "testo": x.Rimozione["testo"]})
	}
	risposta := m{"esito": "scartato", "riproposto": false, "chiave_idempotenza": corpo["chiave_idempotenza"], "allegato_id": f.ID, "stato": "scartato", "versione": f.Versione,
		"rimozioni": rimozioni, "conservazioni": []any{}, "testo": "Scarto registrato; rimozione dallo staging ancora da completare."}
	b.registraFile(bdStr(corpo["chiave_idempotenza"]), impronta, "scarta_file", f.ID, risposta)
	bdJSON(w, http.StatusOK, risposta)
}

func (b *bancoDU) s3(w http.ResponseWriter, r *http.Request) {
	f, corpo, impronta, ok := b.gestoFile(w, r, "altri_file")
	if !ok {
		return
	}
	if b.modo("S3") == "503" {
		bdKo(w, 503, "NON_CONFIGURATO", "La conservazione degli Altri file non è configurata su questo server: niente è cambiato.", m{"motivo": "conservazione"})
		return
	}
	if f.Stato != "attivo" {
		bdKo(w, 409, "FILE_VERSIONE", "Il file non è attivo.", m{"stato": f.Stato, "versione": f.Versione})
		return
	}
	if !b.controllaDipendenze(w, f, corpo, "altri_file") {
		return
	}
	b.conti["S3"]++
	cons := []any{}
	for _, x := range append([]*fileBanco{f}, b.voci(f)...) {
		x.Stato, x.Versione = "altri_file", x.Versione+1
		x.Conservazione = m{"stato": "copiata", "verificato_il": time.Now().UTC(), "bytes": 1024}
		x.AltriFile = m{"autore": "AD", "quando": time.Now().UTC(), "motivo": corpo["motivo"]}
		cons = append(cons, m{"allegato_id": x.ID, "stato": "copiata", "verificato_il": x.Conservazione["verificato_il"], "bytes": 1024})
	}
	risposta := m{"esito": "in_altri_file", "riproposto": false, "chiave_idempotenza": corpo["chiave_idempotenza"], "allegato_id": f.ID, "stato": "altri_file", "versione": f.Versione,
		"conservazioni": cons, "testo": "File messo in Altri file: il contenuto è conservato."}
	b.registraFile(bdStr(corpo["chiave_idempotenza"]), impronta, "altri_file", f.ID, risposta)
	bdJSON(w, http.StatusOK, risposta)
}

func (b *bancoDU) s4(w http.ResponseWriter, r *http.Request) {
	f, corpo, impronta, ok := b.gestoFile(w, r, "riattiva_file")
	if !ok {
		return
	}
	if f.Stato == "scartato" {
		bdKo(w, 422, "FILE_SCARTATO", "Il file è stato scartato: per riaverlo, ricaricalo.", nil)
		return
	}
	if f.Stato != "altri_file" {
		bdKo(w, 409, "FILE_VERSIONE", "Il file è già attivo.", m{"stato": f.Stato, "versione": f.Versione})
		return
	}
	// un archivio: il perimetro delle voci che S1 ha mostrato (MAT FL-10; EM §10.9, S4 con n_voci e impronta_voci)
	if vv := b.voci(f); len(vv) > 0 {
		ids := []any{}
		for _, v := range vv {
			ids = append(ids, v.ID)
		}
		if bdNum(corpo["n_voci"]) != len(vv) || bdStr(corpo["impronta_voci"]) != bdSha(bdCanonico(ids)) {
			bdKo(w, 409, "ANTEPRIMA_SUPERATA", "Il perimetro delle voci dell'archivio è cambiato.", m{"anteprima": "voci_archivio"})
			return
		}
	}
	b.conti["S4"]++
	for _, v := range b.voci(f) {
		if v.Stato == "altri_file" {
			v.Stato, v.Versione = "attivo", v.Versione+1
			v.Conservazione = m{"stato": "da_rilasciare"}
		}
	}
	f.Stato, f.Versione = "attivo", f.Versione+1
	f.Conservazione = m{"stato": "da_rilasciare"}
	risposta := m{"esito": "riattivato", "riproposto": false, "chiave_idempotenza": corpo["chiave_idempotenza"], "allegato_id": f.ID, "stato": "attivo", "versione": f.Versione,
		"byte_ripristinati": false, "analisi": "letto", "conservazione": m{"stato": "da_rilasciare"}, "testo": "File riattivato: il motore lo rilegge."}
	b.registraFile(bdStr(corpo["chiave_idempotenza"]), impronta, "riattiva_file", f.ID, risposta)
	bdJSON(w, http.StatusOK, risposta)
}

func (b *bancoDU) s5(w http.ResponseWriter, r *http.Request) {
	f := b.fileDi(r.PathValue("a"))
	if f == nil {
		bdKo(w, 422, "FUORI_RFQ", "il file non è di questa RFQ", nil)
		return
	}
	if f.Stato == "scartato" {
		bdKo(w, 410, "FILE_SCARTATO", "Il file è stato scartato in questa RFQ: per riaverlo, ricaricalo.", nil)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	// un PDF in anteprima: application/pdf inline, per il visore pdf.js della pagina (EM §10.9, S5); il contenuto è un
	// PDF sintetico di una pagina, uguale per tutti i PDF del banco
	if f.Est == "pdf" && r.URL.Query().Get("modo") != "scarica" {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "inline; filename=\""+f.Nome+"\"")
		_, _ = w.Write(bdPDF())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+f.Nome+"\"")
	_, _ = io.WriteString(w, "contenuto sintetico di "+f.Nome+"\n")
}

// bdPDF: un PDF sintetico di una pagina (un rettangolo), con la tabella xref calcolata: lo legge il visore pdf.js.
func bdPDF() []byte {
	contenuto := "0.1 0.3 0.4 rg 20 20 160 120 re f\n"
	oggetti := []string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R]/Count 1>>",
		"<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 160]/Contents 4 0 R/Resources<<>>>>",
		fmt.Sprintf("<</Length %d>>\nstream\n%sendstream", len(contenuto), contenuto),
	}
	var sb strings.Builder
	sb.WriteString("%PDF-1.4\n")
	posizioni := make([]int, len(oggetti))
	for i, o := range oggetti {
		posizioni[i] = sb.Len()
		fmt.Fprintf(&sb, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := sb.Len()
	fmt.Fprintf(&sb, "xref\n0 %d\n0000000000 65535 f \n", len(oggetti)+1)
	for _, p := range posizioni {
		fmt.Fprintf(&sb, "%010d 00000 n \n", p)
	}
	fmt.Fprintf(&sb, "trailer\n<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(oggetti)+1, xref)
	return []byte(sb.String())
}

// s6: l'anteprima dello STEP (EM §10.9), con lo stesso registro della partenza «step» e, con «bozza», l'effetto del
// cambio della fonte.
func (b *bancoDU) s6(w http.ResponseWriter, r *http.Request) {
	a := r.PathValue("a")
	f := b.fileDi(a)
	if f == nil {
		bdKo(w, 422, "FUORI_RFQ", "lo STEP non è di questa RFQ", nil)
		return
	}
	if f.Est != "stp" && f.Est != "step" {
		bdKo(w, 422, "NON_STEP", "il file non è uno STEP", nil)
		return
	}
	if f.Stato != "attivo" {
		bdKo(w, 422, "FILE_NON_ATTIVO", "lo STEP non è fra i file attivi", nil)
		return
	}
	q := r.URL.Query()
	if q.Get("prodotto") != bdP1 {
		bdJSON(w, http.StatusOK, m{"allegato_id": a, "nome_file": f.Nome, "sha256": f.Sha, "prodotto_id": q.Get("prodotto"), "stato": "struttura_non_determinabile",
			"struttura": m{"versione": 3, "completa": false, "motivo": "lo STEP non propone una struttura per questo prodotto"}, "gruppi": []any{}, "avvisi": []any{}, "testo": ""})
		return
	}
	reg := b.registro(a, f.Sha, b.versioneProposta)
	gruppo := reg["gruppo"].(m)["chiave"]
	letto := "ACM-1000"
	confronto := "coincide"
	if a == bdA10 {
		letto, confronto = "ACM-1000-R2", "diverso"
	}
	// con la candidata incerta (sonda s6-incerta) niente è in vista all'apertura: la proposta arriva solo per il gruppo
	// chiesto (EM §10.9, «in_vista»: mai per un'incerta; [U] RAT05-FONTE-SOSTITUIBILE)
	incerta := b.s6Incerta && a == bdA1
	comp := map[bool]string{false: "compatibile", true: "incerta"}[incerta]
	o := m{"allegato_id": a, "nome_file": f.Nome, "sha256": f.Sha, "prodotto_id": bdP1, "stato": "pronta", "struttura": m{"versione": 3, "completa": true, "motivo": ""},
		"gruppi": []any{m{"gruppo": gruppo, "radice": m{"identita_proposta": m{"letterale": letto, "emittente": "cliente:" + bdCliente}}, "compatibilita": comp, "in_vista": !incerta,
			"nodi": 10, "archi": 9, "segnalati": 3, "impronta_contenuto": reg["impronta_contenuto"]}},
		"codice_prodotto": "ACM-1000", "confronto": confronto,
		"associazioni_previste": []any{m{"allegato_id": bdA2, "chiave_bozza": "k-7"}, m{"allegato_id": bdA3, "chiave_bozza": "k-11"}}, "step_secondario": nil,
		"versioni": m{"importazione": "importazione-1"}, "avvisi": []any{}, "testo": ""}
	if !incerta || q.Get("gruppo") == gruppo {
		o["proposta"] = m{"contenuto": reg, "stato_di_partenza": b.statoDiPartenza(reg, bdImpronta(reg))}
	}
	if bz := q.Get("bozza"); bz != "" {
		bb := b.bozze[bdP1]
		if bb == nil || bb.ID != bz {
			bdKo(w, 422, "FUORI_RFQ", "la bozza non è di questa RFQ", nil)
			return
		}
		if s, _ := bb.Snapshot["sorgente"].(m); bdStr(s["allegato_id"]) == a {
			o["effetto_sulla_bozza"] = m{"stessa_sorgente": true}
		} else {
			o["effetto_sulla_bozza"] = m{"nodi_che_restano": []any{"k-root"}, "nodi_che_entrano": []any{"k-7", "k-9", "k-11"}, "fuori_per_cambio_fonte": []any{"k-12"}, "nuovi_nodi_proposti": []any{},
				"caselle_riscritte": []any{}, "archi_riscritti": []any{}, "identita_aggiornate": []any{}, "da_rivedere": []any{}, "decisioni_cadute": []any{}, "differenze": m{},
				"preassociazioni_nel_vassoio": []any{}}
		}
	}
	bdJSON(w, http.StatusOK, o)
}

func (b *bancoDU) s7(w http.ResponseWriter, r *http.Request) {
	f := b.fileDi(r.PathValue("a"))
	if f == nil {
		bdKo(w, 422, "FUORI_RFQ", "lo STEP non è di questa RFQ", nil)
		return
	}
	corpo, _, err := bdCorpo(r)
	if err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il corpo non è JSON", nil)
		return
	}
	b.corpiFile = append(b.corpiFile, m{"gesto": "associa_step", "allegato": f.ID, "corpo": corpo})
	chiave := bdStr(corpo["chiave_idempotenza"])
	if it, ok := b.intenti[chiave]; ok {
		rr := m{}
		for k, v := range it.Risposta {
			rr[k] = v
		}
		rr["riproposto"] = true
		bdJSON(w, http.StatusOK, rr)
		return
	}
	if len([]rune(bdStr(corpo["motivo"]))) > 500 {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il motivo sta in 500 caratteri", m{"campo": "motivo"})
		return
	}
	if f.Codice != "ACM-1000" && bdStr(corpo["motivo"]) == "" {
		bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "Il codice della proposta non è quello del prodotto: serve un motivo.", m{"campo": "motivo"})
		return
	}
	b.conti["S7"]++
	f.Componente = bdP1
	b.fonte["conferma_ammessa"] = true
	b.fonte["motivo"] = ""
	risposta := m{"esito": "associato", "riproposto": false, "chiave_idempotenza": chiave, "proposta_id": f.Proposta, "testo": "STEP associato a ACM-1000: ora si può confermare la fonte."}
	b.registraFile(chiave, "associa", "associa_step", f.ID, risposta)
	bdJSON(w, http.StatusOK, risposta)
}

// atto: i sei atti esistenti, con la risposta HTML di rispondiMotoreA solo se la richiesta viene dalla pagina
// /thread/{id}/motore-a (dalMotoreA, motore_a.go); altrimenti la «vecchia interfaccia», che il ponte legge «errore».
func (b *bancoDU) atto(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	cur := r.Header.Get("HX-Current-URL")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") != "true" || !strings.HasSuffix(cur, "/thread/"+bdThread+"/motore-a") {
		_, _ = io.WriteString(w, "<main><h1>vecchia interfaccia</h1></main>")
		return
	}
	b.conti["atti"]++
	testo := "Fatto."
	switch {
	case strings.HasSuffix(r.URL.Path, "/fascicolo/conferma"):
		if r.Form.Get("voce") != b.fonte["voce"] || !b.fonte["conferma_ammessa"].(bool) {
			_, _ = fmt.Fprint(w, `<div data-ma-esito="no">La voce non è confermabile: lo STEP non è associato al prodotto.</div>`)
			return
		}
		b.fonte["confermata"] = true
		testo = "Fonte confermata: documento STEP sorgente registrato, copia sul NAS in coda."
	case strings.HasSuffix(r.URL.Path, "/step-strutturale/revoca"):
		b.fonte["autorizzata"] = false
		testo = "Autorizzazione revocata."
	case strings.HasSuffix(r.URL.Path, "/step-strutturale"):
		if r.Form.Get("presa_d_atto") != "1" || r.Form.Get("firma") == "" {
			_, _ = fmt.Fprint(w, `<div data-ma-esito="no">Manca la presa d'atto.</div>`)
			return
		}
		b.fonte["autorizzata"] = true
		testo = "STEP strutturale autorizzato."
	case strings.HasSuffix(r.URL.Path, "/fascicolo/prepara"):
		testo = "Preparazione accodata: 0 download, 0 estrazioni, 0 analisi."
	case strings.HasSuffix(r.URL.Path, "/fascicolo/rianalizza"):
		testo = "Rianalisi accodata."
	case strings.HasSuffix(r.URL.Path, "/riprova-copie"):
		testo = "Copie riaccodate."
	}
	_, _ = fmt.Fprintf(w, `<div data-ma-esito="ok">%s</div>`, testo)
}

// ---------------------------------------------------------------- la sonda

// sonda: GET /__banco/dati (gli identificativi inventati del banco), GET /__banco/stato (lo stato del banco), POST
// /__banco/modo {rotta, esito} (un esito da provocare una volta), POST /__banco/ritardo {rotta, ms} (un'attesa prima
// della risposta, una volta; rotta = il pattern del mux), POST /__banco/f8-errore (F8 in errore fino a R11), POST
// /__banco/proposta (la proposta di oggi cambia), POST /__banco/bozza-altrove (un salvataggio di un collega), POST
// /__banco/interruttore {nome, attivo}, POST /__banco/bozza-formato2, e dall'ondata B POST /__banco/conferma-formato2
// (una conferma formato 2 del secondo prodotto), POST /__banco/bozza-registrato (una bozza «registrato» aperta altrove
// sul primo prodotto), POST /__banco/sostituisci-bozza (la bozza del primo prodotto chiusa e riaperta altrove).
func (b *bancoDU) sonda(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/__banco/dati":
		allegati := m{}
		proposte := m{}
		for _, f := range b.file {
			allegati[f.Nome] = f.ID
			proposte[f.Nome] = f.Proposta
		}
		bdJSON(w, http.StatusOK, m{"thread": bdThread, "utente": duUUID(77).String(), "prodotto": bdP1, "prodotto2": bdP2, "allegati": allegati, "proposte": proposte})
	case r.Method == http.MethodGet && r.URL.Path == "/__banco/stato":
		file := m{}
		for _, f := range b.file {
			file[f.Nome] = m{"stato": f.Stato, "versione": f.Versione, "analisi": f.Analisi, "rimozione": f.Rimozione}
		}
		bozze := m{}
		for p, bz := range b.bozze {
			bozze[p] = m{"bozza_id": bz.ID, "stato": bz.Stato, "versione": bz.Versione, "eventi": len(bz.Eventi), "snapshot": bz.Snapshot}
		}
		conti := m{}
		for k, v := range b.conti {
			conti[k] = v
		}
		bdJSON(w, http.StatusOK, m{"conti": conti, "corpi_r2": b.corpiR2, "chiavi_r5": b.chiaviR5, "chiavi_r6": b.chiaviR6, "corpi_file": b.corpiFile, "conferme": b.conferme, "legacy": b.legacy, "file": file, "bozze": bozze, "fonte": b.fonte})
	case r.Method == http.MethodPost && r.URL.Path == "/__banco/modo":
		var o struct{ Rotta, Esito string }
		_ = json.NewDecoder(r.Body).Decode(&o)
		b.modi[o.Rotta] = o.Esito
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/__banco/ritardo":
		var o struct {
			Rotta string
			Ms    int
		}
		_ = json.NewDecoder(r.Body).Decode(&o)
		b.ritardi[o.Rotta] = time.Duration(o.Ms) * time.Millisecond
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/__banco/f8-errore":
		b.f8Errore = true
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/__banco/proposta":
		b.versioneProposta++
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/__banco/bozza-altrove":
		if bz := b.bozze[bdP1]; bz != nil {
			bz.Versione++
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/__banco/interruttore":
		// {nome, attivo}: congelata (la BOM congelata), s6-incerta (una sola candidata incerta), base-superata (R1 con la
		// base superata fino al rebase), uguale-alla-base (R4 con uguale_alla_base e R5 con NESSUNA_MODIFICA, PE-23),
		// radice-segnalata (il segnale LS-02 sulla voce della radice, P-6)
		var o struct {
			Nome   string
			Attivo bool
		}
		_ = json.NewDecoder(r.Body).Decode(&o)
		switch o.Nome {
		case "congelata":
			b.congelata = o.Attivo
		case "s6-incerta":
			b.s6Incerta = o.Attivo
		case "base-superata":
			b.baseSuperata = o.Attivo
		case "uguale-alla-base":
			b.segnoUguale = o.Attivo
		case "radice-segnalata":
			// anche nei registri già scritti: la bozza aperta li rilegge con R1 ?importazione=1
			b.radiceSegnalata = o.Attivo
			for _, reg := range b.registri {
				for _, x := range reg["proposta"].(m)["nodi"].([]any) {
					if v := x.(m); bdStr(v["origine"].(m)["chiave"]) == "#1" {
						if o.Attivo {
							v["segnali"] = bdSegnaleRadice()
						} else {
							v["segnali"] = []any{}
						}
					}
				}
			}
		default:
			http.Error(w, "interruttore sconosciuto", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/__banco/conferma-formato2":
		// una conferma formato 2 del secondo prodotto, l'ultima e valida (come le scriveva la pagina vecchia): la prima
		// conferma formato 3 di un prodotto resta ammessa anche se è uguale a lei (PE-23)
		n := len(b.conferme[bdP2]) + 1
		for _, c := range b.conferme[bdP2] {
			c["ultima"] = false
		}
		b.conferme[bdP2] = append(b.conferme[bdP2], m{"conferma_id": fmt.Sprintf("00000000-0000-4000-8000-0000000f2%03d", n), "numero": n, "formato": 2, "senza_riepilogo_v2": false,
			"ultima": true, "valida": true, "confermato_da": duUUID(77).String(), "confermato_da_nome": "AD", "confermato_il": "2026-10-12T09:00:00Z", "firma_struttura": strings.Repeat("2", 64),
			"criterio_verifica": "nessuno", "mono_pezzo": false, "origine": m{"rotta": "distinta/albero/conferma"}})
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && (r.URL.Path == "/__banco/bozza-registrato" || r.URL.Path == "/__banco/sostituisci-bozza"):
		// bozza-registrato: una bozza aperta del primo prodotto con la partenza «registrato», aperta da un'altra pagina
		// (la pagina nuova non la offre con una conferma formato 3, PE-23: la bozza c'è lo stesso); il prodotto non deve
		// averne una aperta. sostituisci-bozza: la bozza aperta del primo prodotto si chiude altrove e se ne apre una nuova
		// dalla conferma (le modifiche tenute da parte nella pagina erano della bozza di prima)
		vecchia := b.bozze[bdP1]
		aperta := vecchia != nil && vecchia.Stato == "aperta"
		if (r.URL.Path == "/__banco/bozza-registrato") == aperta {
			http.Error(w, "bozza del prodotto non nello stato atteso", http.StatusConflict)
			return
		}
		partenza := "registrato"
		if aperta {
			vecchia.Stato = "abbandonata"
			partenza = "conferma"
		}
		b.nBozze++
		bz := &bozzaBanco{ID: fmt.Sprintf("00000000-0000-4000-8000-0000000b%04d", b.nBozze), Stato: "aperta", Versione: 1, Creata: time.Now().UTC()}
		b.costruisciDaRegistrato(bz, bdP1, partenza)
		b.bozze[bdP1] = bz
		b.eventoServer(bz, "bozza_aperta", m{"partenza": partenza})
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/__banco/bozza-formato2":
		// una bozza del formato di prima, aperta, per il primo prodotto (EM §6.7): solo se il prodotto non ne ha una aperta
		if bz := b.bozze[bdP1]; bz != nil && bz.Stato == "aperta" {
			http.Error(w, "il prodotto ha già una bozza aperta", http.StatusConflict)
			return
		}
		b.nBozze++
		bz := &bozzaBanco{ID: fmt.Sprintf("00000000-0000-4000-8000-0000000b%04d", b.nBozze), Stato: "aperta", Versione: 3, Creata: time.Now().UTC(), Formato2: true,
			Snapshot: m{"formato": 2, "prodotto_id": bdP1, "nodi": []any{m{"chiave_bozza": "k-root", "componente_id": bdP1, "op": "riusa", "tipo": "finito", "codice": "ACM-1000", "rev": "C", "origine": "registrato"}},
				"archi": []any{}, "decisioni": []any{}, "documenti_preassociati": []any{}}}
		b.bozze[bdP1] = bz
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// bdOrdini: gli ordinamenti del JSON canonico di EM §3.10, per campo.
var bdOrdini = map[string][]string{"nodi": {"chiave_bozza"}, "archi": {"padre", "figlio"}, "decisioni": {"id"}, "documenti_preassociati": {"proposta_id"},
	"caselle": {"gruppo", "rif", "allegato_id"}}

// bdFormaCanonica: la forma del JSON canonico di EM §3.10, come la salva il server: facoltativi assenti, vuoti o null
// tolti; nodi[], archi[], decisioni[], documenti_preassociati[] e caselle[] ordinati. La NFC di descrizione e motivi no:
// i testi del banco e delle prove sono già NFC. Firma e impronte del banco si calcolano su questa forma.
func bdFormaCanonica(v any, chiave string) any {
	switch x := v.(type) {
	case m:
		out := m{}
		for k, vv := range x {
			c := bdFormaCanonica(vv, k)
			if !bdVuoto(c) {
				out[k] = c
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = bdFormaCanonica(e, "")
		}
		if campi, ok := bdOrdini[chiave]; ok {
			sort.SliceStable(out, func(i, j int) bool { return bdChiaveOrdine(out[i], campi) < bdChiaveOrdine(out[j], campi) })
		}
		return out
	}
	return v
}

func bdVuoto(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case m:
		return len(x) == 0
	}
	return false
}

func bdChiaveOrdine(v any, campi []string) string {
	o, _ := v.(m)
	parti := make([]string, len(campi))
	for i, c := range campi {
		parti[i] = bdStr(o[c])
	}
	return strings.Join(parti, "\x00")
}

// bdSnapshotCanonico: lo snapshot nella forma canonica, con i dodici campi di primo livello che R1 restituisce anche
// vuoti (EM §3.2): nodi, archi, decisioni e documenti preassociati restano elenchi, anche senza elementi.
func bdSnapshotCanonico(s m) m {
	out, _ := bdFormaCanonica(s, "").(m)
	if out == nil {
		out = m{}
	}
	for _, k := range []string{"nodi", "archi", "decisioni", "documenti_preassociati"} {
		if _, ok := out[k]; !ok {
			out[k] = []any{}
		}
	}
	return out
}

// bdCanonico: JSON con le chiavi ordinate (per le impronte sintetiche del banco).
func bdCanonico(v any) string {
	switch x := v.(type) {
	case m:
		k := make([]string, 0, len(x))
		for kk := range x {
			k = append(k, kk)
		}
		sort.Strings(k)
		var sb strings.Builder
		sb.WriteString("{")
		for i, kk := range k {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(strconv.Quote(kk) + ":" + bdCanonico(x[kk]))
		}
		sb.WriteString("}")
		return sb.String()
	case []any:
		parti := make([]string, len(x))
		for i, e := range x {
			parti[i] = bdCanonico(e)
		}
		return "[" + strings.Join(parti, ",") + "]"
	case json.Number:
		return x.String()
	default:
		bb, _ := json.Marshal(x)
		return string(bb)
	}
}

// ---------------------------------------------------------------- L1: il banco serve le forme del contratto

// bdChiedi: una richiesta al banco, con il corpo JSON e il codice atteso; torna l'oggetto della risposta.
func (b *bancoDU) bdChiedi(t *testing.T, metodo, percorso string, corpo any, atteso int) m {
	t.Helper()
	var rd io.Reader
	if corpo != nil {
		bb, _ := json.Marshal(corpo)
		rd = bytes.NewReader(bb)
	}
	req, _ := http.NewRequest(metodo, b.srv.URL+percorso, rd)
	req.Header.Set("Accept", "application/json")
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var o m
	_ = json.NewDecoder(resp.Body).Decode(&o)
	if resp.StatusCode != atteso {
		t.Fatalf("%s %s: %d, atteso %d (%v)", metodo, percorso, resp.StatusCode, atteso, o)
	}
	return o
}

func bdHaCampi(t *testing.T, dove string, o m, campi ...string) {
	t.Helper()
	for _, c := range campi {
		if _, ok := o[c]; !ok {
			t.Errorf("%s: manca il campo %q del contratto", dove, c)
		}
	}
}

// TestBancoDistintaUnicaConforme: le risposte finte del banco hanno i campi che EM §9 e §10.9 danno alle rotte di V1, e
// i flussi principali (scelta dello STEP, salvataggio con la chiave, riepilogo, conferma con l'intento, gesti dei file)
// si comportano come il contratto dice: è la condizione perché la prova nel browser sul banco dica qualcosa del ponte.
// Non prova il server: le risposte sono finte.
func TestBancoDistintaUnicaConforme(t *testing.T) {
	b := nuovoBancoDU(t)
	base := "/thread/" + bdThread
	d := b.bdChiedi(t, "GET", base+"/motore-a/dati", nil, 200)
	bdHaCampi(t, "/dati", d, "thread_id", "testa", "esito", "errore", "gruppi", "fonte", "f8_stato")
	r1 := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1, nil, 200)
	if r1["stato"] != "nessuna_bozza" {
		t.Fatalf("R1 all'inizio: %v", r1["stato"])
	}
	bdHaCampi(t, "R1 senza bozza", r1, "stato", "versione_corrente")
	s1 := b.bdChiedi(t, "GET", base+"/sorgente?scartati=1", nil, 200)
	bdHaCampi(t, "S1", s1, "thread_id", "firma", "configurazione", "contatori", "file", "step", "dettaglio")
	bdHaCampi(t, "S1 file", s1["file"].([]any)[0].(m), "allegato_id", "proposta_id", "nome_file", "estensione", "sha256", "bytes", "origine", "canale", "stato", "versione", "proposta", "analisi", "byte", "rimozione", "conservazione", "azioni")
	s6 := b.bdChiedi(t, "GET", base+"/sorgente/step/"+bdA1+"/anteprima?prodotto="+bdP1, nil, 200)
	bdHaCampi(t, "S6", s6, "allegato_id", "nome_file", "sha256", "prodotto_id", "stato", "struttura", "gruppi", "codice_prodotto", "confronto", "proposta", "associazioni_previste", "step_secondario", "versioni", "avvisi", "testo")
	g := s6["gruppi"].([]any)[0].(m)
	bdHaCampi(t, "S6 gruppo", g, "gruppo", "radice", "compatibilita", "in_vista", "nodi", "archi", "segnalati", "impronta_contenuto")
	// R2 crea con partenza step: la versione 1 la costruisce il server, la risposta porta lo snapshot
	chiave := "00000000-0000-4000-8000-0000000e0001"
	crea := m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": chiave,
		"crea": m{"partenza": "step", "sorgente": m{"allegato_id": bdA1, "sha256": bdShaStep, "gruppo": g["gruppo"]}, "impronta_contenuto": g["impronta_contenuto"]}}
	r2 := b.bdChiedi(t, "POST", base+"/motore-a/bozza", crea, 200)
	bdHaCampi(t, "R2 crea", r2, "bozza_id", "formato", "versione", "nuova_versione", "riproposto", "eventi", "impronta", "salvata_il", "snapshot", "importazione")
	// lo stesso corpo con la stessa chiave: riproposto; un corpo diverso con la stessa chiave: 409 CHIAVE_RIUSATA
	if rr := b.bdChiedi(t, "POST", base+"/motore-a/bozza", crea, 200); rr["riproposto"] != true {
		t.Errorf("R2 reinviato con la stessa chiave: riproposto %v", rr["riproposto"])
	}
	crea["crea"].(m)["motivo"] = "altro corpo"
	if rr := b.bdChiedi(t, "POST", base+"/motore-a/bozza", crea, 409); rr["codice"] != "CHIAVE_RIUSATA" {
		t.Errorf("R2 con la stessa chiave e un altro corpo: %v", rr)
	}
	r1 = b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1+"&eventi=1&importazione=1", nil, 200)
	bdHaCampi(t, "R1 aperta", r1, "stato", "formato", "vocabolario", "bozza_id", "versione", "snapshot", "impronta", "chiave_ultimo_salvataggio", "importazione", "importazioni_citate", "eventi",
		"versione_corrente", "base", "rif_scaduti", "vassoio")
	if _, ok := r1["importazione"].(m)["contenuto"]; !ok {
		t.Error("R1 ?importazione=1 senza il contenuto del registro")
	}
	snap := r1["snapshot"].(m)
	bdHaCampi(t, "snapshot formato 3", snap, "formato", "prodotto_id", "sorgente", "versione_bom_base", "firma_base", "nodi", "archi", "decisioni", "documenti_preassociati")
	// R4 e R5: la firma del riepilogo va alla conferma; la conferma registra l'intento, R6 lo ritrova
	corpoBozza := m{}
	for k, v := range snap {
		corpoBozza[k] = v
	}
	corpoBozza["bozza_id"], corpoBozza["bozza_versione"] = r1["bozza_id"], r1["versione"]
	r4 := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo?avvisi_motore=1", corpoBozza, 200)
	bdHaCampi(t, "R4", r4, "formato", "prodotto_id", "versione_corrente", "base", "rif_scaduti", "identita", "archi", "non_inclusi", "relazioni", "documenti_preassociati", "svincoli",
		"blocchi", "avvisi", "avvisi_motore", "avvisi_motore_calcolati", "registro_decisioni", "criterio_previsto", "confermabile", "firma_anteprima", "importazione")
	if r4["confermabile"] != false {
		t.Error("R4 sulla bozza appena creata: atteso non confermabile (CODICE_DA_ASSEGNARE del pezzo senza codice)")
	}
	conf := b.bdChiedi(t, "POST", base+"/motore-a/bom/conferma", m{"firma_anteprima": r4["firma_anteprima"], "bozza": func() m {
		c := m{}
		for k, v := range corpoBozza {
			c[k] = v
		}
		c["chiave_idempotenza"] = "00000000-0000-4000-8000-0000000e0002"
		return c
	}()}, 422)
	if conf["codice"] != "BLOCCHI" {
		t.Errorf("R5 con i blocchi: %v", conf)
	}
	r6 := b.bdChiedi(t, "GET", base+"/motore-a/intento/00000000-0000-4000-8000-0000000e0099", nil, 200)
	if r6["stato"] != "non_registrato" {
		t.Errorf("R6 di una chiave che non c'è: %v", r6)
	}
	r7 := b.bdChiedi(t, "GET", base+"/motore-a/conferme?prodotto="+bdP1, nil, 200)
	bdHaCampi(t, "R7", r7, "prodotto_id", "conferme", "attestazioni", "fonte")
	// S2 su un file che la bozza usa come fonte: 422 DIPENDENZE_ATTIVE; S3 di un file libero: in_altri_file
	det := b.bdChiedi(t, "GET", base+"/sorgente?file="+bdA1, nil, 200)["dettaglio"].(m)
	if rr := b.bdChiedi(t, "POST", base+"/sorgente/file/"+bdA1+"/scarta", m{"versione_attesa": 0, "chiave_idempotenza": "00000000-0000-4000-8000-0000000e0003", "motivo": "prova",
		"impronta_dipendenze": det["impronta_dipendenze"]}, 422); rr["codice"] != "DIPENDENZE_ATTIVE" {
		t.Errorf("S2 sulla fonte della bozza: %v", rr)
	}
	det = b.bdChiedi(t, "GET", base+"/sorgente?file="+bdA5, nil, 200)["dettaglio"].(m)
	s3 := b.bdChiedi(t, "POST", base+"/sorgente/file/"+bdA5+"/altri-file", m{"versione_attesa": 0, "chiave_idempotenza": "00000000-0000-4000-8000-0000000e0004",
		"impronta_dipendenze": det["impronta_dipendenze"]}, 200)
	bdHaCampi(t, "S3", s3, "esito", "riproposto", "chiave_idempotenza", "allegato_id", "stato", "versione", "conservazioni", "testo")
	if rr := b.bdChiedi(t, "GET", base+"/motore-a/intento/00000000-0000-4000-8000-0000000e0004", nil, 200); rr["stato"] != "registrato" {
		t.Errorf("R6 dopo S3: %v", rr)
	}
	// nessun percorso del disco nelle risposte (T-15, sul banco)
	for _, p := range []string{base + "/sorgente?scartati=1", base + "/motore-a/bozza?prodotto=" + bdP1 + "&importazione=1"} {
		req, _ := http.NewRequest("GET", b.srv.URL+p, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		bb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if regexp.MustCompile(`[A-Za-z]:\\\\|\\\\\\\\`).Match(bb) {
			t.Errorf("%s: un percorso del disco nella risposta", p)
		}
	}
	// il guscio vero si apre dal banco, e una rotta legacy finisce nell'elenco della sonda
	req, _ := http.NewRequest("GET", b.srv.URL+base+"/distinta-unica", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	bb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(bb), `data-du-thread="`+bdThread+`"`) {
		t.Errorf("il guscio dal banco: %d", resp.StatusCode)
	}
	resp, err = http.Post(b.srv.URL+base+"/distinta/albero/conferma", "application/json", strings.NewReader("{}"))
	if err == nil {
		resp.Body.Close()
	}
	if st := b.bdChiedi(t, "GET", "/__banco/stato", nil, 200); len(st["legacy"].([]any)) != 1 {
		t.Errorf("la sonda non ha visto la chiamata legacy: %v", st["legacy"])
	}
}

// TestBancoDistintaUnicaOndataB: le risposte finte che l'ondata B aggiunge al banco, con le forme delle decisioni
// dell'ondata A: la variante di R1 per l'effetto del cambio di tipo (PE-09), uguale_alla_base di R4 e NESSUNA_MODIFICA
// di R5 con l'interruttore (PE-23; spento, il banco non li dà, come il server di oggi), la conferma formato 2, il segnale
// sulla radice (P-6) e il caricamento lasciato cadere della prova del tempo d'inattività. Non prova il server.
func TestBancoDistintaUnicaOndataB(t *testing.T) {
	b := nuovoBancoDU(t)
	base := "/thread/" + bdThread
	// PE-09: {componente_id, tipo_nuovo, effetto, firma_effetto}; un cambio che non si fa ha effetto.spento
	e := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1+"&effetto_cambio_tipo="+bdC40+"&tipo_nuovo=sciolto", nil, 200)
	bdHaCampi(t, "effetto del cambio di tipo", e, "componente_id", "tipo_nuovo", "effetto", "firma_effetto")
	if !regexp.MustCompile(`^[0-9a-f]{24,64}$`).MatchString(bdStr(e["firma_effetto"])) || e["tipo_nuovo"] != "sciolto" || bdStr(e["effetto"].(m)["spento"]) != "" {
		t.Errorf("effetto del cambio di tipo: %v", e)
	}
	if e := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1+"&effetto_cambio_tipo="+bdC40+"&tipo_nuovo=commerciale", nil, 200); bdStr(e["effetto"].(m)["spento"]) == "" {
		t.Errorf("un cambio che non si fa senza effetto.spento: %v", e)
	}
	b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1+"&effetto_cambio_tipo="+bdP2+"&tipo_nuovo=sciolto", nil, 422)
	b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1+"&effetto_cambio_tipo="+bdC40+"&tipo_nuovo=pezzo", nil, 422)
	// PE-23 sul secondo prodotto: una conferma formato 2, poi la bozza dalla conferma
	b.bdChiedi(t, "POST", "/__banco/conferma-formato2", nil, 204)
	n := 0
	crea := func(partenza string) m {
		n++
		r2 := b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP2, "vocabolario": 2,
			"chiave_salvataggio": fmt.Sprintf("00000000-0000-4000-8000-0000000e1%03d", n), "crea": m{"partenza": partenza}}, 200)
		c := m{}
		for k, v := range r2["snapshot"].(m) {
			c[k] = v
		}
		c["bozza_id"], c["bozza_versione"] = r2["bozza_id"], r2["versione"]
		return c
	}
	corpo := crea("conferma")
	if r4 := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo", corpo, 200); r4["uguale_alla_base"] != nil {
		t.Errorf("R4 con l'interruttore spento dà uguale_alla_base: %v", r4["uguale_alla_base"])
	}
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "uguale-alla-base", "attivo": true}, 204)
	r4 := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo", corpo, 200)
	if r4["uguale_alla_base"] != false {
		t.Errorf("R4 su una base formato 2: uguale_alla_base %v, atteso falso (la prima conferma formato 3 è ammessa)", r4["uguale_alla_base"])
	}
	conferma := func(c m, r4 m, chiave string, atteso int) m {
		cc := m{}
		for k, v := range c {
			cc[k] = v
		}
		cc["chiave_idempotenza"] = chiave
		return b.bdChiedi(t, "POST", base+"/motore-a/bom/conferma", m{"firma_anteprima": r4["firma_anteprima"], "bozza": cc}, atteso)
	}
	conferma(corpo, r4, "00000000-0000-4000-8000-0000000e2001", 200)
	corpo = crea("conferma")
	r4 = b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo", corpo, 200)
	if r4["uguale_alla_base"] != true {
		t.Errorf("R4 sulla bozza dalla conferma formato 3, senza modifiche: uguale_alla_base %v", r4["uguale_alla_base"])
	}
	if rr := conferma(corpo, r4, "00000000-0000-4000-8000-0000000e2002", 422); rr["codice"] != "NESSUNA_MODIFICA" {
		t.Errorf("R5 di una bozza uguale alla base: %v", rr)
	}
	corpo["nodi"].([]any)[0].(m)["descrizione"] = "Nastro di carico, rivisto"
	if r4 := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo", corpo, 200); r4["uguale_alla_base"] != false {
		t.Errorf("R4 di una bozza cambiata: uguale_alla_base %v", r4["uguale_alla_base"])
	}
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "uguale-alla-base", "attivo": false}, 204)
	if r7 := b.bdChiedi(t, "GET", base+"/motore-a/conferme?prodotto="+bdP2, nil, 200); len(r7["conferme"].([]any)) != 2 {
		t.Errorf("conferme del secondo prodotto: %v", r7["conferme"])
	}
	// P-6: il segnale sulla radice, anche nel registro di una bozza già aperta
	s6 := b.bdChiedi(t, "GET", base+"/sorgente/step/"+bdA1+"/anteprima?prodotto="+bdP1, nil, 200)
	g := s6["gruppi"].([]any)[0].(m)
	b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": "00000000-0000-4000-8000-0000000e3001",
		"crea": m{"partenza": "step", "sorgente": m{"allegato_id": bdA1, "sha256": bdShaStep, "gruppo": g["gruppo"]}, "impronta_contenuto": g["impronta_contenuto"]}}, 200)
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "radice-segnalata", "attivo": true}, 204)
	r1 := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1+"&importazione=1", nil, 200)
	radice := r1["importazione"].(m)["contenuto"].(m)["proposta"].(m)["nodi"].([]any)[0].(m)
	if len(radice["segnali"].([]any)) != 1 {
		t.Errorf("la voce della radice senza il segnale: %v", radice["segnali"])
	}
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "radice-segnalata", "attivo": false}, 204)
	// la prova del tempo d'inattività: il caricamento lasciato cadere non registra niente
	b.bdChiedi(t, "POST", "/__banco/modo", m{"rotta": "E1", "esito": "appeso"}, 204)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("origine", "portale")
	fw, _ := mw.CreateFormFile("file", "LENTO_DI_PROVA.pdf")
	_, _ = fw.Write([]byte("%PDF-1.4 prova"))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", b.srv.URL+base+"/carica", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	st := b.bdChiedi(t, "GET", "/__banco/stato", nil, 200)
	if resp.StatusCode != 503 || st["file"].(m)["LENTO_DI_PROVA.pdf"] != nil {
		t.Errorf("il caricamento lasciato cadere: %d, file nel banco %v", resp.StatusCode, st["file"].(m)["LENTO_DI_PROVA.pdf"])
	}
}
