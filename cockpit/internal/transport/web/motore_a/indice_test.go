package motore_a

// L1 — Giro 5, A1d (BE-17; contratto A1d §2.3; v1.1 §4.3; X-02…X-06, X-09; T-A1d-09, T-A1d-10, T-A1d-19…T-A1d-27):
// l'indice della Distinta nuova sulla scena ACME (scenaIndiceACME). Ogni ricerca del contratto e ogni ricerca in più;
// un figlio con due padri, che compare sotto tutti e due; «nel file» e «confermato» accanto dopo uno spostamento, con
// Figli[].Decisa e Legami.Confermati[].NelFile che dicono la stessa cosa per lo stesso arco (D-PF-02, RAC-09); le chiavi
// dei gruppi stabili e opache (X-02); i conflitti per asse; l'indice nil, vuoto o senza fotografia, che risponde con il
// valore zero. Nessun DB: la fotografia e l'esito si costruiscono in Go, nei tipi veri (P7e).
//
// Derivate dalle prove della PR #18 di Stefano Valo (67dcc02: motore_a_indice_test.go), con i nomi del sottopackage e
// le correzioni di A1d-P2 (PD-02 della revisione di A1d-P2).
//
// I clienti di questi test sono inventati: ACME, codici 712xxxx, UUID …0000000000nn, sha di 64 cifre.

import (
	"fmt"
	"reflect"
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

// ---- gli aiuti ACME piccoli (RAC-20: nascono qui, Q3 li riusa) ----

// maUUID: l'UUID ACME «00000000-0000-4000-8000-0000000000nn» (nn in cifre decimali).
func maUUID(nn int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", nn))
}

// maUUIDp: lo stesso UUID, come puntatore nuovo.
func maUUIDp(nn int) *uuid.UUID {
	u := maUUID(nn)
	return &u
}

// maSha: uno sha256 inventato di 64 cifre; maSha(1) < maSha(2) < …
func maSha(n int) string { return fmt.Sprintf("%064d", n) }

func maInt(v int) *int { return &v }

func maStr(s string) *string { return &s }

// maNodo: il Rif del nodo con quella chiave nel contenuto maSha(sha) (ancoraggio.RifNodo, mai a mano).
func maNodo(sha int, chiave string) string { return ancoraggio.RifNodo(maSha(sha), chiave) }

// maComp: il Rif «componente:<uuid>» del componente nn (ancoraggio.RifComponente).
func maComp(nn int) string { return ancoraggio.RifComponente(maUUID(nn)) }

// maP1, maP2: i Rif dei due prodotti della scena.
var maP1, maP2 = maComp(10), maComp(20)

// ---- la scena ACME dell'indice (piano di A1d backend §7.1) ----

// scenaIndiceACME: la fotografia, l'esito (valutazione-3) e il confronto di una RFQ ACME inventata, costruiti nei tipi
// veri. I numeri sono le ultime cifre degli UUID; S1…S7 sono maSha(1)…maSha(7).
//   - thread 01 (APERTA, «RFQ 7120001», «ACME-030P7120001»), cliente 02 «ACME S.p.A.», messaggio 03, utente 09;
//   - componenti: 10 7120001 finito (P1, descrizione registrata «Gruppo ACME», STEP strutturale 41), 11 7120010
//     sottoassieme (rev B, senza descrizione), 12 7120011 sciolto, 13 7120012 commerciale archiviato, 14 7120013 sciolto,
//     15 7120099 finito (un padre fuori dal perimetro di P1), 20 7120002 finito (P2);
//   - relazioni confermate: 10→11 ×2, 11→12 ×4, 12→14 ×2, 15→11 ×1 (lo «Sposta»: 11 confermato anche sotto 7120099,
//     che il file non ha);
//   - allegati: 31 «7120001A_1.stp» S1; 32 «7120001A_1 copia.stp» S1 (lo stesso contenuto); 33 «7120005.stp» S2 (due
//     radici candidate); 34 «7120010_B.pdf» S3 (il 2D di 11); 35 «7120011.pdf» S4 (da smistare, non 2D nell'esito); 36
//     «Capitolato ACME.docx» S5; 37 «7120002.stp» S1 (di P2); 38 «senza_sha.pdf» senza sha256;
//   - documenti: 41 cad_3d di 10 da 31, corrente, scritto; 42 2D di 11 sostituito da 43; 43 2D di 11 rev B da 34,
//     corrente, in coda; 44 cad_3d di 12, corrente, in errore;
//   - proposte: 51 di 35 aperta; 52 di 34 confermata;
//   - righe di componente_proposta: 61 (31, S1, #1, confermata, 10), 62 (31, S1, #3, aperta), 63 (33, S2, #1), 64 (33,
//     S2, #2); la riga 61 serve alla chiave della radice (AutorizzazioneDiStruttura): nell'esito il nodo #1 resta senza
//     Decisione, così si prova la regola della radice della BOM di lavoro (T-A1d-22);
//   - righe di relazione_proposta: (31, #1, #2) e (31, #1, #3) aperte; (32, #1, #3) aperta (lo stesso contenuto in due
//     allegati di P1); (37, #1, #4) aperta (lo stesso sha in un allegato di P2); (31, #2, #3) confermata;
//   - rimozione aperta: step 41, 10→11, qta 2;
//   - P1: BOM di lavoro da 31 (#1 radice senza Decisione; #2 deciso 11 sotto #1 ×2; #3 sotto #1 ×8 e #2 ×1, due padri,
//     senza un file ancorato); candidate SC1 (33, radice #1 del file: #1, #2 sotto #1 ×1), SC2 (33, radice interna #2:
//     #2, #3 sotto #2 ×3; nodo:S2:#2 sta in SC1 e in SC2, D6), SC3 (32, i nodi come la BOM: Rif doppi); confermati senza
//     STEP 12 (sotto nodo:S1:#2 ×4) e 14 (sotto componente:…12 ×2); voci (11, 2D, 43 in coda, primario 34), (12, 3D, 44
//     in errore), (10, 3D, 41 scritto); candidati della fonte (31, S1), (33, S2), (36, S5);
//   - P2: una candidata da 37;
//   - conflitti: c1 arco rimozione dello step 41 (10→11), c2 lo stesso arco per la quantità (con il documento 41
//     nell'evidenza: è il motivo, non il documento, che fa la rimozione), c3 rimozione dello step 44, c4 associazione
//     sullo smistamento, c5 nuovo_file senza prodotto;
//   - confronto: 34 uguale, 35 nuovo_ancoraggio.
//
// L'ordine delle candidate (SC1, SC2, SC3) è quello dell'esito anche se non è quello di ancoraggio (per sha256): l'indice
// non riordina niente.
func scenaIndiceACME() (fotorfq.Fotografia, valutazione.Esito, confronto.Esito) {
	presa := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	utente, messaggio := maUUID(9), maUUID(3)

	componente := func(nn int, codice, tipo string) fotorfq.Componente {
		return fotorfq.Componente{ID: maUUID(nn), Codice: codice, Tipo: tipo, Origine: "manuale", ConfermatoDa: utente, CreatoIl: presa}
	}
	c10 := componente(10, "7120001", "finito")
	c10.Descrizione, c10.StepStrutturaleID = maStr("Gruppo ACME"), maUUIDp(41)
	c11 := componente(11, "7120010", "sottoassieme")
	c11.Rev = maStr("B")
	c13 := componente(13, "7120012", "commerciale")
	archiviato := presa.Add(-time.Hour)
	c13.ArchiviatoIl = &archiviato

	allegato := func(nn int, nome string, sha int) fotorfq.Allegato {
		a := fotorfq.Allegato{ID: maUUID(nn), MessaggioID: messaggio, Indice: int16(nn - 30), NomeFile: nome, Natura: "file",
			Stato: "analizzato", RicevutoIl: presa}
		if sha > 0 {
			a.Sha256 = maStr(maSha(sha))
		}
		return a
	}
	a34 := allegato(34, "7120010_B.pdf", 3)
	a34.Estensione = maStr("pdf")

	documento := func(nn, comp int, tipo, nome string, sha int, nas string, allegati ...int) fotorfq.DocumentoConfermato {
		d := fotorfq.DocumentoConfermato{ID: maUUID(nn), ThreadID: maUUID(1), ComponenteID: maUUIDp(comp), Tipo: tipo, NomeFile: nome,
			Estensione: nome[strings.LastIndex(nome, ".")+1:], Sha256: maSha(sha), StatoNas: nas, ConfermatoDa: utente, ConfermatoIl: presa}
		for _, a := range allegati {
			d.Allegati = append(d.Allegati, maUUID(a))
		}
		return d
	}
	d42 := documento(42, 11, "disegno_2d", "7120010_A.pdf", 6, "scritto")
	d42.SostituitoDa = maUUIDp(43)
	d43 := documento(43, 11, "disegno_2d", "7120010_B.pdf", 3, "in_coda", 34)
	d43.Rev = maStr("B")

	rigaComponente := func(nn, all, sha int, chiave, stato string, comp *uuid.UUID) fotorfq.RigaComponenteProposta {
		return fotorfq.RigaComponenteProposta{ID: maUUID(nn), AllegatoID: maUUID(all), Sha256: maSha(sha), Chiave: chiave,
			IDGrezzo: chiave, Fonte: "step", Stato: stato, ComponenteID: comp}
	}
	rigaRelazione := func(all int, padre, figlio string, qta int, stato string) fotorfq.RigaRelazioneProposta {
		return fotorfq.RigaRelazioneProposta{AllegatoID: maUUID(all), PadreChiave: padre, FiglioChiave: figlio, Qta: qta, Stato: stato}
	}

	th := fotorfq.Thread{ID: maUUID(1), ClienteID: maUUID(2), Stato: string(db.StatoThreadAPERTA), Oggetto: maStr("RFQ 7120001"),
		Riferimento: maStr("ACME-030P7120001"), CreatoIl: presa,
		Allegati: []fotorfq.Allegato{allegato(31, "7120001A_1.stp", 1), allegato(32, "7120001A_1 copia.stp", 1),
			allegato(33, "7120005.stp", 2), a34, allegato(35, "7120011.pdf", 4), allegato(36, "Capitolato ACME.docx", 5),
			allegato(37, "7120002.stp", 1), allegato(38, "senza_sha.pdf", 0)},
		Proposte: []fotorfq.PropostaAttuale{
			{ID: maUUID(51), AllegatoID: maUUID(35), ThreadID: maUUIDp(1), Tipo: "disegno_2d", Fonte: "nome_file", Stato: string(db.StatoPropostaAperta)},
			{ID: maUUID(52), AllegatoID: maUUID(34), ThreadID: maUUIDp(1), Tipo: "disegno_2d", Fonte: "nome_file", Stato: string(db.StatoPropostaConfermata),
				ComponenteID: maUUIDp(11)},
		},
		Documenti: []fotorfq.DocumentoConfermato{documento(41, 10, "cad_3d", "7120001A_1.stp", 1, "scritto", 31), d42, d43,
			documento(44, 12, "cad_3d", "7120011.stp", 7, "errore")},
		Componenti: []fotorfq.Componente{c10, c11, componente(12, "7120011", "sciolto"), c13, componente(14, "7120013", "sciolto"),
			componente(15, "7120099", "finito"), componente(20, "7120002", "finito")},
		Relazioni: []fotorfq.Relazione{
			{PadreID: maUUID(10), FiglioID: maUUID(11), Qta: 2, Origine: "manuale", ConfermatoDa: utente, CreatoIl: presa},
			{PadreID: maUUID(11), FiglioID: maUUID(12), Qta: 4, Origine: "manuale", ConfermatoDa: utente, CreatoIl: presa},
			{PadreID: maUUID(12), FiglioID: maUUID(14), Qta: 2, Origine: "manuale", ConfermatoDa: utente, CreatoIl: presa},
			{PadreID: maUUID(15), FiglioID: maUUID(11), Qta: 1, Origine: "manuale", ConfermatoDa: utente, CreatoIl: presa},
		},
		RigheComponenteProposta: []fotorfq.RigaComponenteProposta{
			rigaComponente(61, 31, 1, "#1", string(db.StatoPropostaConfermata), maUUIDp(10)),
			rigaComponente(62, 31, 1, "#3", string(db.StatoPropostaAperta), nil),
			rigaComponente(63, 33, 2, "#1", string(db.StatoPropostaAperta), nil),
			rigaComponente(64, 33, 2, "#2", string(db.StatoPropostaAperta), nil),
		},
		RigheRelazioneProposta: []fotorfq.RigaRelazioneProposta{
			rigaRelazione(31, "#1", "#2", 2, string(db.StatoPropostaAperta)),
			rigaRelazione(31, "#1", "#3", 8, string(db.StatoPropostaAperta)),
			rigaRelazione(32, "#1", "#3", 8, string(db.StatoPropostaAperta)),
			rigaRelazione(37, "#1", "#4", 1, string(db.StatoPropostaAperta)),
			rigaRelazione(31, "#2", "#3", 1, string(db.StatoPropostaConfermata)),
		},
		RimozioniAperte: []fotorfq.RimozioneAperta{{StepDocumentoID: maUUID(41), PadreID: maUUID(10), FiglioID: maUUID(11), QtaWorking: 2}},
	}
	for i := range th.RigheComponenteProposta {
		for _, a := range th.Allegati {
			if a.ID == th.RigheComponenteProposta[i].AllegatoID {
				th.RigheComponenteProposta[i].NomeFile = a.NomeFile
			}
		}
	}
	for i := range th.RigheRelazioneProposta {
		for _, a := range th.Allegati {
			if a.ID == th.RigheRelazioneProposta[i].AllegatoID {
				th.RigheRelazioneProposta[i].NomeFile = a.NomeFile
			}
		}
	}
	f := fotorfq.Fotografia{VersioneSchema: fotorfq.VersioneSchema, Origine: fotorfq.OrigineDSN, PresaIl: presa, Coerente: true,
		Clienti: []fotorfq.Cliente{{ID: maUUID(2), RagioneSociale: "ACME S.p.A."}}, Thread: []fotorfq.Thread{th}}

	// ---- l'esito ----
	parentela := func(padre string, q int, decisa bool) valutazione.Parentela {
		return valutazione.Parentela{Padre: padre, Quantita: maInt(q), Decisa: decisa}
	}
	nodo := func(all, sha int, chiave string, padri ...valutazione.Parentela) valutazione.NodoBOM {
		return valutazione.NodoBOM{Nodo: ancoraggio.NodoProposto{Rif: maNodo(sha, chiave), AllegatoID: maUUID(all), EntitaID: chiave,
			Leggibile: true}, Parentela: padri}
	}
	deciso := func(nn int, codice string) *ancoraggio.ComponenteDeciso {
		return &ancoraggio.ComponenteDeciso{ComponenteID: maUUID(nn), Codice: codice, Autorita: ancoraggio.AutoritaConfermata,
			Origine: ancoraggio.OrigineConfermato}
	}
	d34 := valutazione.Disegno2D{DocumentoID: maUUIDp(43), AllegatoID: maUUIDp(34), Sha256: maSha(3), NomeFile: "7120010_B.pdf",
		Formato: valutazione.Formato2DPDF, Estensione: "pdf", Validita: valutazione.ValiditaValido, Provenienza: ancoraggio.OrigineConfermato,
		Corrente: true, Anteprima: valutazione.RiferimentoAnteprima{Sha256: maSha(3), Formato: valutazione.Formato2DPDF,
			DocumentoID: maUUIDp(43), AllegatoID: maUUIDp(34), Disponibile: true}}

	n1 := nodo(31, 1, "#1")
	n1.Tipo, n1.OrigineTipo = "finito", valutazione.OrigineTipoConfermato
	n2 := nodo(31, 1, "#2", parentela(maNodo(1, "#1"), 2, true))
	n2.Nodo.Decisione, n2.Tipo, n2.OrigineTipo = deciso(11, "7120010"), "sottoassieme", valutazione.OrigineTipoConfermato
	n2.Disegni = valutazione.GruppoDisegni2D{Primario: &d34}
	n3 := nodo(31, 1, "#3", parentela(maNodo(1, "#1"), 8, false), parentela(maNodo(1, "#2"), 1, false))
	n3.Nodo.SenzaFile = true // un nodo dello STEP senza un file ancorato: la sua Parentela resta la quantità dei fatti
	n3.Nodo.RigaLegacy = &ancoraggio.RigaPropostaLegacy{ID: maUUID(62), AllegatoID: maUUID(31), Sha256: maSha(1), Chiave: "#3",
		Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto}
	n3.Tipo, n3.OrigineTipo = "sciolto", valutazione.OrigineTipoPropostoDallaRegola

	senzaSTEP := func(nn int, codice, tipo string, padri ...valutazione.Parentela) valutazione.NodoBOM {
		return valutazione.NodoBOM{Nodo: ancoraggio.NodoProposto{Rif: maComp(nn), SenzaFile: true, Decisione: deciso(nn, codice)},
			Parentela: padri, Tipo: tipo, OrigineTipo: valutazione.OrigineTipoConfermato}
	}
	candidata := func(all, sha int, radice string, delFile bool, nodi ...valutazione.NodoBOM) valutazione.StrutturaCandidata {
		return valutazione.StrutturaCandidata{AllegatoID: maUUID(all), Sha256: maSha(sha), Radice: radice, RadiceDelFile: delFile,
			Compatibilita: motorea.CompatibilitaUguale, GrafoCompleto: true, Nodi: nodi}
	}
	sc2 := candidata(33, 2, maNodo(2, "#2"), false, nodo(33, 2, "#2"), nodo(33, 2, "#3", parentela(maNodo(2, "#2"), 3, false)))
	sc2.Compatibilita = motorea.CompatibilitaParziale

	p1 := valutazione.ProdottoValutato{Rif: maP1, Autorita: ancoraggio.AutoritaConfermata, ComponenteID: maUUIDp(10), CodiceRichiesto: "7120001",
		Identita: valutazione.IdentitaConfermata, Struttura: ancoraggio.StatoBOMDiLavoroProposta,
		Fonte: valutazione.FonteProdotto{Stato: valutazione.FonteConfermata, Motivo: valutazione.MotivoFonteEstrazioneRiuscita, Calcolata: true,
			Candidati: []ancoraggio.DocumentoCandidato{
				{Origine: ancoraggio.CandidatoDaMotoreA, AllegatoID: maUUIDp(31), Sha256: maSha(1), Radici: []string{"#1"}, Estrazione: ancoraggio.EstrazioneRiuscita},
				{Origine: ancoraggio.CandidatoDaMotoreA, AllegatoID: maUUIDp(33), Sha256: maSha(2), Radici: []string{"#1"}, Estrazione: ancoraggio.EstrazioneRiuscita},
				{Origine: ancoraggio.CandidatoDaPropostaAperta, AllegatoID: maUUIDp(36), Sha256: maSha(5), Radici: []string{"#9"},
					Estrazione: ancoraggio.EstrazioneNonAnalizzata},
			}},
		Nodi: []valutazione.NodoBOM{n1, n2, n3},
		StruttureCandidate: []valutazione.StrutturaCandidata{
			candidata(33, 2, maNodo(2, "#1"), true, nodo(33, 2, "#1"), nodo(33, 2, "#2", parentela(maNodo(2, "#1"), 1, false))),
			sc2,
			candidata(32, 1, maNodo(1, "#1"), true, nodo(32, 1, "#1"), nodo(32, 1, "#2", parentela(maNodo(1, "#1"), 2, false)),
				nodo(32, 1, "#3", parentela(maNodo(1, "#1"), 8, false), parentela(maNodo(1, "#2"), 1, false))),
		},
		ConfermatiSenzaSTEP: []valutazione.NodoBOM{
			senzaSTEP(12, "7120011", "sciolto", parentela(maNodo(1, "#2"), 4, true)),
			senzaSTEP(14, "7120013", "sciolto", parentela(maComp(12), 2, true)),
		},
		Documenti: valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Voci: []valutazione.VoceFabbisogno{
			{ComponenteID: maUUID(11), Codice: "7120010", TipoComponente: "sottoassieme", TipoDocumento: "disegno_2d", Bloccante: true,
				Esito: valutazione.EsitoPresente, Disegni: &valutazione.GruppoDisegni2D{Primario: &d34}, DocumentoID: maUUIDp(43), StatoNas: "in_coda"},
			{ComponenteID: maUUID(12), Codice: "7120011", TipoComponente: "sciolto", TipoDocumento: "cad_3d", Bloccante: true,
				Esito: valutazione.EsitoPresente, DocumentoID: maUUIDp(44), StatoNas: "errore"},
			{ComponenteID: maUUID(10), Codice: "7120001", TipoComponente: "finito", DelProdotto: true, TipoDocumento: "cad_3d", Bloccante: true,
				Esito: valutazione.EsitoPresente, DocumentoID: maUUIDp(41), StatoNas: "scritto"},
		}},
		Stato: valutazione.ProdottoNonPronto,
	}
	p2 := valutazione.ProdottoValutato{Rif: maP2, Autorita: ancoraggio.AutoritaConfermata, ComponenteID: maUUIDp(20), CodiceRichiesto: "7120002",
		Identita: valutazione.IdentitaConfermata, Struttura: ancoraggio.StatoStrutturaCandidata,
		StruttureCandidate: []valutazione.StrutturaCandidata{candidata(37, 1, maNodo(1, "#1"), true, nodo(37, 1, "#1"))},
		Stato:              valutazione.ProdottoNonPronto}

	arco := func(padre, figlio string, q int) ancoraggio.ArcoProposto {
		return ancoraggio.ArcoProposto{Padre: padre, Figlio: figlio, Quantita: maInt(q)}
	}
	struttura := func(target string, all, sha int, radice string, delFile bool, stato ancoraggio.StatoStruttura,
		archi ...ancoraggio.ArcoProposto) ancoraggio.StrutturaProdotto {
		return ancoraggio.StrutturaProdotto{Target: target, AllegatoID: maUUID(all), Sha256: maSha(sha), Radice: radice, RadiceDelFile: delFile,
			Compatibilita: motorea.CompatibilitaUguale, Stato: stato, Archi: archi, GrafoCompleto: true}
	}
	bom12 := arco(maNodo(1, "#1"), maNodo(1, "#2"), 2)
	bom12.Decisione = &ancoraggio.ArcoPercorso{Padre: maComp(10), Figlio: maComp(11), Quantita: maInt(2), Origine: ancoraggio.OrigineArcoConfermato}
	strutture := []ancoraggio.StrutturaProdotto{
		struttura(maP1, 31, 1, maNodo(1, "#1"), true, ancoraggio.StatoBOMDiLavoroProposta, bom12,
			arco(maNodo(1, "#1"), maNodo(1, "#3"), 8), arco(maNodo(1, "#2"), maNodo(1, "#3"), 1)),
		struttura(maP1, 32, 1, maNodo(1, "#1"), true, ancoraggio.StatoStrutturaCandidata, arco(maNodo(1, "#1"), maNodo(1, "#2"), 2),
			arco(maNodo(1, "#1"), maNodo(1, "#3"), 8), arco(maNodo(1, "#2"), maNodo(1, "#3"), 1)),
		struttura(maP1, 33, 2, maNodo(2, "#1"), true, ancoraggio.StatoStrutturaCandidata, arco(maNodo(2, "#1"), maNodo(2, "#2"), 1)),
		struttura(maP1, 33, 2, maNodo(2, "#2"), false, ancoraggio.StatoStrutturaCandidata, arco(maNodo(2, "#2"), maNodo(2, "#3"), 3)),
		struttura(maP2, 37, 1, maNodo(1, "#1"), true, ancoraggio.StatoStrutturaCandidata),
	}

	arco1011 := valutazione.RifArco(maComp(10), maComp(11))
	conflitti := []valutazione.Conflitto{
		{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia, Rif: arco1011, Prodotto: maP1, Decisione: "qta 2",
			OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: valutazione.MotivoConflittoRimozione, Motivo: valutazione.MotivoConflittoRimozione,
			EvidenzaProposta: valutazione.EvidenzaProposta{DocumentoID: maUUIDp(41)}},
		{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia, Rif: arco1011, Prodotto: maP1, Decisione: "qta 2",
			OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: "qta 3", Motivo: valutazione.MotivoConflittoQuantita,
			EvidenzaProposta: valutazione.EvidenzaProposta{AllegatoID: maUUIDp(31), DocumentoID: maUUIDp(41)}},
		{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia, Rif: arco1011, Prodotto: maP1, Decisione: "qta 2",
			OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: valutazione.MotivoConflittoRimozione, Motivo: valutazione.MotivoConflittoRimozione,
			EvidenzaProposta: valutazione.EvidenzaProposta{DocumentoID: maUUIDp(44)}},
		{Tipo: valutazione.ConflittoAssociazione, Asse: valutazione.AsseSmistamento, Rif: maComp(11), Prodotto: maP1,
			OrigineDecisione: ancoraggio.OrigineConfermato, Motivo: valutazione.MotivoConflittoComponenteDiverso,
			EvidenzaProposta: valutazione.EvidenzaProposta{AllegatoID: maUUIDp(34)}},
		{Tipo: valutazione.ConflittoNuovoFile, Asse: valutazione.AsseSmistamento, Rif: maComp(12), Motivo: "disegno_2d",
			OrigineDecisione: ancoraggio.OrigineConfermato, EvidenzaProposta: valutazione.EvidenzaProposta{AllegatoID: maUUIDp(35)}},
	}

	et := valutazione.EsitoThread{ThreadID: maUUID(1), ClienteID: maUUID(2), Valutato: true,
		Ancoraggi:        ancoraggio.EsitoAncoraggi{Strutture: strutture},
		ProdottiValutati: []valutazione.ProdottoValutato{p1, p2},
		DaSmistare: []valutazione.FileDaSmistare{{AllegatoID: maUUID(35), NomeFile: "7120011.pdf",
			Motivo: valutazione.MotivoSmistamentoFileDaSmistare, Prodotti: []string{maP1}, ProdottiContesto: []string{maP2}}},
		Conflitti: conflitti,
		Fascicolo: valutazione.StatoFascicolo{FaseThread: string(db.StatoThreadAPERTA)},
	}
	for _, a := range th.Allegati {
		disp := ancoraggio.DisponibilitaDisponibile
		if a.Sha256 == nil {
			disp = ancoraggio.DisponibilitaMancante
		}
		et.File = append(et.File, valutazione.FileInterpretato{AllegatoID: a.ID, Disponibilita: disp,
			Disegno: strings.HasSuffix(a.NomeFile, ".pdf")})
		as := valutazione.AssociazioneFile{AllegatoID: a.ID, Associazione: ancoraggio.AssociazioneCandidatoUnico,
			Collocazione: ancoraggio.CollocazioneRadice, Perimetro: valutazione.PerimetroDentro}
		if a.ID == maUUID(35) {
			as.Associazione, as.Collocazione = ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile
		}
		et.Associazioni = append(et.Associazioni, as)
	}
	e := valutazione.Esito{VersioneValutazione: valutazione.VersioneValutazione, Thread: []valutazione.EsitoThread{et}}
	c := confronto.Esito{VersioneConfronto: confronto.VersioneConfronto, File: []confronto.EsitoFile{
		{File: confronto.File{AllegatoID: maUUID(34)}, Badge: confronto.BadgeUguale},
		{File: confronto.File{AllegatoID: maUUID(35)}, Badge: confronto.BadgeNuovoAncoraggio},
	}}
	return f, e, c
}

// maIndiceACME: l'indice sulla scena ACME, con il thread e l'esito da cui è costruito (i puntatori che restituisce sono
// quelli: le prove li confrontano per identità).
func maIndiceACME() (*Indice, *fotorfq.Thread, *valutazione.EsitoThread) {
	f, e, c := scenaIndiceACME()
	return nuovoIndice3(&f.Thread[0], &e.Thread[0], &c), &f.Thread[0], &e.Thread[0]
}

// nuovoIndice3: l'indice senza le dichiarazioni dello STEP (le prove della PR #18, che non le avevano).
func nuovoIndice3(t *fotorfq.Thread, e *valutazione.EsitoThread, c *confronto.Esito) *Indice {
	return NuovoIndice(t, e, c, nil)
}

// maChiaveCand: la chiave attesa del gruppo di una candidata, scritta qui per esteso (X-02), non con chiaveCandidata.
func maChiaveCand(all int, radice string) string {
	return "cand:" + maUUID(all).String() + ":" + radice
}

// maQ: una quantità per i messaggi («nil» o il valore).
func maQ(v *int) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprint(*v)
}

// ---- le prove ----

// Le 29 ricerche (11 del contratto §2.3, 14 della v1.1 §4.3, 4 di A1d-P2), né una di più né una di meno: il template le
// chiama per nome.
func TestIndiceMotoreAHaLeVentinoveRicerche(t *testing.T) {
	attese := []string{
		"File", "NomeFile", "Associazione", "Badge", "Prodotto", "Nodo", "QuantitaConfermata", "PropostaAperta",
		"AutorizzazioneDi", "RelazioneProposta", "Rimozione",
		"Gruppi", "NodoIn", "Radici", "Figli", "Legami", "Componente", "Correnti", "Anteprima", "Conflitti",
		"ConflittiDelThread", "Voce", "CopieAperte", "CandidatiFuori", "AutorizzazioneDiStruttura",
		// A1d-P2: le autorizzazioni dello STEP (F-16) e il componente del nodo in un punto solo (F-15)
		"ComponenteDelNodo", "Autorizzazioni", "Marcatura", "StepProdotto",
	}
	tipo := reflect.TypeOf((*Indice)(nil))
	var visti []string
	for i := 0; i < tipo.NumMethod(); i++ {
		visti = append(visti, tipo.Method(i).Name)
	}
	vuoi := map[string]bool{}
	for _, n := range attese {
		vuoi[n] = true
	}
	for _, n := range visti {
		if !vuoi[n] {
			t.Errorf("ricerca in più, fuori dal piano: %s", n)
		}
		delete(vuoi, n)
	}
	for n := range vuoi {
		t.Errorf("ricerca mancante: %s", n)
	}
	if len(attese) != 29 {
		t.Fatalf("l'elenco della prova ha %d nomi", len(attese))
	}
}

// Un indice nil, un indice vuoto e un esito nil rispondono con il valore zero, senza panic; senza fotografia e senza
// confronto rispondono vuoto le ricerche che li usano, le altre no.
func TestIndiceMotoreANilSicuro(t *testing.T) {
	if nuovoIndice3(nil, nil, nil) != nil {
		t.Fatal("nuovoIndice3(nil, nil, nil) non è nil")
	}
	f, e, c := scenaIndiceACME()
	if nuovoIndice3(&f.Thread[0], nil, &c) != nil {
		t.Fatal("senza esito l'indice non è nil")
	}
	tipo := reflect.TypeOf((*Indice)(nil))
	for nome, x := range map[string]*Indice{"nil": nil, "vuoto": {}} {
		v := reflect.ValueOf(x)
		for i := 0; i < tipo.NumMethod(); i++ {
			m := tipo.Method(i)
			args := make([]reflect.Value, m.Type.NumIn()-1)
			for j := range args {
				args[j] = reflect.Zero(m.Type.In(j + 1))
			}
			var out []reflect.Value
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("indice %s, %s: panic %v", nome, m.Name, r)
					}
				}()
				out = v.Method(i).Call(args)
			}()
			for _, o := range out {
				if !o.IsZero() {
					t.Errorf("indice %s, %s: %v, atteso il valore zero", nome, m.Name, o)
				}
			}
		}
	}
	// Senza fotografia (t nil) e senza confronto (c nil).
	x := nuovoIndice3(nil, &e.Thread[0], nil)
	conflitto := e.Thread[0].Conflitti[0]
	switch {
	case x.NomeFile(maUUID(31)) != "", x.PropostaAperta(maUUID(35)) != nil, x.Badge(maUUID(34)) != nil,
		x.RelazioneProposta(maP1, maNodo(1, "#1"), maNodo(1, "#2")) != nil, x.Rimozione(conflitto) != nil,
		x.Componente(maUUID(11)) != nil, x.Correnti(maUUID(11), "disegno_2d") != nil,
		x.AutorizzazioneDi(maP1, maUUID(31)) != nil, x.AutorizzazioneDiStruttura(maP1, maUUID(33), maNodo(2, "#2")) != nil,
		x.Anteprima(maUUID(35)) != nil, x.Legami(maP1, "bom", maNodo(1, "#2")).Confermati != nil:
		t.Error("senza fotografia o senza confronto una ricerca che li usa risponde")
	}
	if x.Prodotto(maP1) == nil || len(x.Gruppi(maP1)) != 5 || x.Anteprima(maUUID(34)) == nil || x.File(maUUID(31)) == nil ||
		len(x.Legami(maP1, "bom", maNodo(1, "#2")).DelFile) != 1 || len(x.Conflitti(maP1, "")) != 4 {
		t.Error("senza fotografia le ricerche sull'esito devono rispondere lo stesso")
	}
}

// Gli 11 metodi del contratto §2.3 (le autorizzazioni, le relazioni e le rimozioni hanno le loro prove).
func TestIndiceMotoreARicercheDelContratto(t *testing.T) {
	x, th, et := maIndiceACME()
	for i := range et.File {
		if x.File(et.File[i].AllegatoID) != &et.File[i] {
			t.Errorf("File(%s) non è il record dell'esito", et.File[i].AllegatoID)
		}
	}
	if x.File(maUUID(99)) != nil || x.Associazione(maUUID(99)) != nil || x.Badge(maUUID(31)) != nil {
		t.Error("un allegato assente, o senza badge, deve dare nil")
	}
	for nn, nome := range map[int]string{31: "7120001A_1.stp", 36: "Capitolato ACME.docx", 38: "senza_sha.pdf", 99: ""} {
		if got := x.NomeFile(maUUID(nn)); got != nome {
			t.Errorf("NomeFile(%d) = %q, atteso %q", nn, got, nome)
		}
	}
	if a := x.Associazione(maUUID(35)); a != &et.Associazioni[4] || a.Associazione != ancoraggio.AssociazioneNessunCandidato {
		t.Errorf("Associazione(35) = %+v", a)
	}
	if b := x.Badge(maUUID(34)); b == nil || b.Badge != confronto.BadgeUguale {
		t.Errorf("Badge(34) = %+v", b)
	}
	if b := x.Badge(maUUID(35)); b == nil || b.Badge != confronto.BadgeNuovoAncoraggio {
		t.Errorf("Badge(35) = %+v", b)
	}
	if x.Prodotto(maP1) != &et.ProdottiValutati[0] || x.Prodotto(maP2) != &et.ProdottiValutati[1] || x.Prodotto("identificativo:7120003") != nil {
		t.Error("Prodotto non trova i prodotti per Rif")
	}
	// PropostaAperta: il documento_proposta.id aperto del file, una copia; una proposta confermata non lo è.
	if p := x.PropostaAperta(maUUID(35)); p == nil || *p != maUUID(51) {
		t.Errorf("PropostaAperta(35) = %v, attesa la 51", p)
	} else {
		*p = uuid.Nil
		if th.Proposte[0].ID != maUUID(51) {
			t.Error("PropostaAperta restituisce la memoria della fotografia")
		}
	}
	if p := x.PropostaAperta(maUUID(34)); p != nil {
		t.Errorf("PropostaAperta(34) = %v: la proposta è confermata", p)
	}
	// QuantitaConfermata, nel gruppo: dalla decisione dell'arco della struttura del gruppo; per un confermato senza STEP
	// dalla sua Parentela (bom e senza); altrimenti nil. La candidata SC3 (allegato 32) ha gli stessi Rif della BOM di
	// lavoro (lo stesso sha256), ma non la sua quantità confermata (IX-01 della revisione di A1d-P2).
	sc3 := maChiaveCand(32, maNodo(1, "#1"))
	for _, c := range []struct {
		prodotto, gruppo, padre, figlio string
		attesa                          *int
	}{
		{maP1, "bom", maNodo(1, "#1"), maNodo(1, "#2"), maInt(2)},
		{maP1, "bom", maNodo(1, "#2"), maComp(12), maInt(4)},
		{maP1, "bom", maComp(12), maComp(14), maInt(2)},
		{maP1, "senza", maComp(12), maComp(14), maInt(2)},
		{maP1, "senza", maNodo(1, "#1"), maNodo(1, "#2"), nil},
		{maP1, "bom", maNodo(1, "#1"), maNodo(1, "#3"), nil},
		{maP1, "bom", maNodo(1, "#2"), maNodo(1, "#3"), nil},
		{maP1, sc3, maNodo(1, "#1"), maNodo(1, "#2"), nil},
		{maP1, sc3, maNodo(1, "#2"), maComp(12), nil},
		{maP1, "boh", maNodo(1, "#1"), maNodo(1, "#2"), nil},
		{maP2, "bom", maNodo(1, "#1"), maNodo(1, "#2"), nil},
	} {
		if got := x.QuantitaConfermata(c.prodotto, c.gruppo, c.padre, c.figlio); maQ(got) != maQ(c.attesa) {
			t.Errorf("QuantitaConfermata(%s, %s, %s, %s) = %s, attesa %s", c.prodotto, c.gruppo, c.padre, c.figlio, maQ(got), maQ(c.attesa))
		}
	}
	q := x.QuantitaConfermata(maP1, "bom", maNodo(1, "#1"), maNodo(1, "#2"))
	*q = 99
	if *et.Ancoraggi.Strutture[0].Archi[0].Decisione.Quantita != 2 {
		t.Error("QuantitaConfermata restituisce la memoria dell'esito")
	}
}

// Nodo dà il primo nodo con quel Rif (T-A1d-10); NodoIn lo cerca nel solo gruppo (D6 di P7e).
func TestIndiceMotoreANodoEIlPrimoNodoInDistingue(t *testing.T) {
	x, _, et := maIndiceACME()
	p1 := &et.ProdottiValutati[0]
	sc1, sc2, sc3 := &p1.StruttureCandidate[0], &p1.StruttureCandidate[1], &p1.StruttureCandidate[2]
	if n := x.Nodo(maP1, maNodo(2, "#2")); n != &sc1.Nodi[1] {
		t.Errorf("Nodo(P1, nodo:S2:#2) = %p, atteso quello di SC1 (%p)", n, &sc1.Nodi[1])
	}
	if n := x.NodoIn(maP1, maChiaveCand(33, maNodo(2, "#2")), maNodo(2, "#2")); n != &sc2.Nodi[0] || len(n.Parentela) != 0 {
		t.Errorf("NodoIn(P1, SC2, nodo:S2:#2) = %p, atteso la radice di SC2 (%p)", n, &sc2.Nodi[0])
	}
	if n := x.NodoIn(maP1, maChiaveCand(33, maNodo(2, "#2")), maNodo(2, "#1")); n != nil {
		t.Errorf("NodoIn di un Rif fuori dal gruppo = %+v", n.Nodo.Rif)
	}
	if x.Nodo(maP1, maNodo(1, "#1")) != &p1.Nodi[0] || x.NodoIn(maP1, maChiaveCand(32, maNodo(1, "#1")), maNodo(1, "#1")) != &sc3.Nodi[0] {
		t.Error("lo stesso Rif nella BOM e in SC3: Nodo dà quello della BOM, NodoIn quello del gruppo")
	}
	if x.Nodo(maP1, maComp(12)) != &p1.ConfermatiSenzaSTEP[0] || x.NodoIn(maP1, "bom", maComp(14)) != &p1.ConfermatiSenzaSTEP[1] {
		t.Error("i confermati senza STEP si trovano con Nodo e, nella scheda bom, con NodoIn")
	}
	if x.Nodo(maP1, maNodo(9, "#1")) != nil || x.NodoIn(maP1, "boh", maNodo(1, "#1")) != nil || x.Nodo(maP2, maNodo(1, "#2")) != nil {
		t.Error("un Rif o un gruppo che non c'è deve dare nil")
	}
}

// Gruppi: bom, le candidate nell'ordine dell'esito, senza; nessun campo che dica «scelta» (T-A1d-09, T-A1d-27, X-03).
func TestIndiceMotoreAGruppiSenzaPreselezione(t *testing.T) {
	x, _, et := maIndiceACME()
	p1 := &et.ProdottiValutati[0]
	g := x.Gruppi(maP1)
	type atteso struct {
		chiave, tipo, radice, nome string
		allegato                   int
		struttura                  *valutazione.StrutturaCandidata
		nodi                       []*valutazione.NodoBOM
	}
	nodiDi := func(nn []valutazione.NodoBOM) []*valutazione.NodoBOM {
		var out []*valutazione.NodoBOM
		for i := range nn {
			out = append(out, &nn[i])
		}
		return out
	}
	attesi := []atteso{
		{"bom", "bom", maNodo(1, "#1"), "7120001A_1.stp", 31, nil,
			append(nodiDi(p1.Nodi), &p1.ConfermatiSenzaSTEP[0], &p1.ConfermatiSenzaSTEP[1])},
		{maChiaveCand(33, maNodo(2, "#1")), "cand", maNodo(2, "#1"), "7120005.stp", 33, &p1.StruttureCandidate[0], nodiDi(p1.StruttureCandidate[0].Nodi)},
		{maChiaveCand(33, maNodo(2, "#2")), "cand", maNodo(2, "#2"), "7120005.stp", 33, &p1.StruttureCandidate[1], nodiDi(p1.StruttureCandidate[1].Nodi)},
		{maChiaveCand(32, maNodo(1, "#1")), "cand", maNodo(1, "#1"), "7120001A_1 copia.stp", 32, &p1.StruttureCandidate[2], nodiDi(p1.StruttureCandidate[2].Nodi)},
		{"senza", "senza", "", "", 0, nil, nodiDi(p1.ConfermatiSenzaSTEP)},
	}
	if len(g) != len(attesi) {
		t.Fatalf("Gruppi(P1): %d gruppi, attesi %d", len(g), len(attesi))
	}
	for i, a := range attesi {
		v := g[i]
		if v.Chiave != a.chiave || v.Tipo != a.tipo || v.Radice != a.radice || v.NomeFile != a.nome || v.Struttura != a.struttura {
			t.Errorf("gruppo %d: %q %q %q %q %p, atteso %q %q %q %q %p", i, v.Chiave, v.Tipo, v.Radice, v.NomeFile, v.Struttura,
				a.chiave, a.tipo, a.radice, a.nome, a.struttura)
		}
		if (a.allegato == 0) != (v.AllegatoID == nil) || (v.AllegatoID != nil && *v.AllegatoID != maUUID(a.allegato)) {
			t.Errorf("gruppo %q: AllegatoID %v, atteso %d", v.Chiave, v.AllegatoID, a.allegato)
		}
		if !reflect.DeepEqual(v.Nodi, a.nodi) {
			var r []string
			for _, n := range v.Nodi {
				r = append(r, n.Nodo.Rif)
			}
			t.Errorf("gruppo %q: nodi %v", v.Chiave, r)
		}
	}
	// Nessun campo di scelta: i campi di Gruppo sono questi e basta.
	var campi []string
	for i := 0; i < reflect.TypeOf(Gruppo{}).NumField(); i++ {
		campi = append(campi, reflect.TypeOf(Gruppo{}).Field(i).Name)
	}
	if strings.Join(campi, " ") != "Chiave Tipo AllegatoID Radice NomeFile Struttura Nodi" {
		t.Errorf("i campi di Gruppo sono %v", campi)
	}
	// P2: la sola candidata; un prodotto che non c'è: niente. La cache dà lo stesso elenco.
	if g2 := x.Gruppi(maP2); len(g2) != 1 || g2[0].Chiave != maChiaveCand(37, maNodo(1, "#1")) || g2[0].NomeFile != "7120002.stp" {
		t.Errorf("Gruppi(P2) = %+v", g2)
	}
	if x.Gruppi("identificativo:7120003") != nil {
		t.Error("Gruppi di un prodotto che non c'è")
	}
	if g2 := x.Gruppi(maP1); &g2[0] != &g[0] {
		t.Error("la seconda chiamata non viene dalla cache")
	}
	// Senza BOM di lavoro la scheda bom non c'è; senza confermati senza STEP nemmeno «senza».
	p1.Nodi, p1.ConfermatiSenzaSTEP = nil, nil
	y := nuovoIndice3(nil, et, nil)
	if g := y.Gruppi(maP1); len(g) != 3 || g[0].Tipo != "cand" || g[2].Tipo != "cand" {
		t.Errorf("Gruppi senza BOM e senza confermati = %d gruppi", len(g))
	}
}

// X-02: le chiavi dei gruppi sono opache e stabili. Dipendono solo dall'allegato e dalla radice: con le candidate in un
// altro ordine ogni gruppo tiene la sua chiave e i suoi nodi, e l'ordine delle schede è quello del nuovo esito.
func TestIndiceMotoreALeChiaviDeiGruppiSonoStabili(t *testing.T) {
	x, _, _ := maIndiceACME()
	prima := map[string]string{}
	for _, g := range x.Gruppi(maP1) {
		if g.Tipo == "cand" {
			if g.AllegatoID == nil || g.Chiave != "cand:"+g.AllegatoID.String()+":"+g.Radice {
				t.Errorf("la chiave %q non è «cand:<allegato>:<radice>»", g.Chiave)
			}
			prima[g.Chiave] = g.Nodi[0].Nodo.Rif + "@" + g.Nodi[0].Nodo.AllegatoID.String()
		}
	}
	f, e, c := scenaIndiceACME()
	sc := e.Thread[0].ProdottiValutati[0].StruttureCandidate
	sc[0], sc[2] = sc[2], sc[0]
	y := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c)
	g := y.Gruppi(maP1)
	if len(g) != 5 || g[1].Chiave != maChiaveCand(32, maNodo(1, "#1")) || g[3].Chiave != maChiaveCand(33, maNodo(2, "#1")) {
		t.Fatalf("con le candidate in un altro ordine: %v %v", g[1].Chiave, g[3].Chiave)
	}
	for _, v := range g {
		if v.Tipo != "cand" {
			continue
		}
		if prima[v.Chiave] != v.Nodi[0].Nodo.Rif+"@"+v.Nodi[0].Nodo.AllegatoID.String() {
			t.Errorf("la chiave %q ora porta a un'altra candidata", v.Chiave)
		}
		if n := y.NodoIn(maP1, v.Chiave, v.Radice); n == nil || n.Nodo.AllegatoID != *v.AllegatoID {
			t.Errorf("NodoIn con la chiave %q non trova la radice della sua candidata", v.Chiave)
		}
	}
}

// Radici e Figli: un figlio con due padri compare sotto tutti e due (contratto §1.6.1); la quantità dei fatti e quella
// confermata stanno accanto; per un confermato senza STEP la quantità dei fatti non c'è (T-A1d-20), mentre un nodo dello
// STEP senza un file ancorato la tiene.
func TestIndiceMotoreARadiciEFigli(t *testing.T) {
	x, _, et := maIndiceACME()
	p1 := &et.ProdottiValutati[0]
	if r := x.Radici(maP1, "bom"); len(r) != 1 || r[0] != &p1.Nodi[0] {
		t.Errorf("Radici(bom) = %d nodi", len(r))
	}
	if r := x.Radici(maP1, "senza"); len(r) != 1 || r[0] != &p1.ConfermatiSenzaSTEP[0] {
		t.Errorf("Radici(senza) = %d nodi", len(r))
	}
	if r := x.Radici(maP1, maChiaveCand(33, maNodo(2, "#2"))); len(r) != 1 || r[0] != &p1.StruttureCandidate[1].Nodi[0] {
		t.Errorf("Radici(SC2) = %d nodi", len(r))
	}
	if x.Radici(maP1, "boh") != nil {
		t.Error("Radici di un gruppo che non c'è")
	}
	type figlio struct {
		nodo          *valutazione.NodoBOM
		q, confermata *int
		decisa        bool
	}
	for _, c := range []struct {
		gruppo, padre string
		attesi        []figlio
	}{
		{"bom", maNodo(1, "#1"), []figlio{{&p1.Nodi[1], maInt(2), maInt(2), true}, {&p1.Nodi[2], maInt(8), nil, false}}},
		{"bom", maNodo(1, "#2"), []figlio{{&p1.Nodi[2], maInt(1), nil, false}, {&p1.ConfermatiSenzaSTEP[0], nil, maInt(4), true}}},
		{"bom", maComp(12), []figlio{{&p1.ConfermatiSenzaSTEP[1], nil, maInt(2), true}}},
		{"senza", maComp(12), []figlio{{&p1.ConfermatiSenzaSTEP[1], nil, maInt(2), true}}},
		{"bom", maNodo(1, "#3"), nil},
		{maChiaveCand(33, maNodo(2, "#2")), maNodo(2, "#2"), []figlio{{&p1.StruttureCandidate[1].Nodi[1], maInt(3), nil, false}}},
	} {
		got := x.Figli(maP1, c.gruppo, c.padre)
		if len(got) != len(c.attesi) {
			t.Errorf("Figli(%s, %s): %d figli, attesi %d", c.gruppo, c.padre, len(got), len(c.attesi))
			continue
		}
		for i, a := range c.attesi {
			g := got[i]
			if g.Nodo != a.nodo || maQ(g.Quantita) != maQ(a.q) || maQ(g.QuantitaConfermata) != maQ(a.confermata) || g.Decisa != a.decisa {
				t.Errorf("Figli(%s, %s)[%d] = %s ×%s ✓%s decisa=%v, atteso %s ×%s ✓%s decisa=%v", c.gruppo, c.padre, i,
					g.Nodo.Nodo.Rif, maQ(g.Quantita), maQ(g.QuantitaConfermata), g.Decisa,
					a.nodo.Nodo.Rif, maQ(a.q), maQ(a.confermata), a.decisa)
			}
		}
	}
	// La quantità di Figli è una copia.
	if f := x.Figli(maP1, "bom", maNodo(1, "#1")); f[0].Quantita != nil {
		*f[0].Quantita = 99
		if *p1.Nodi[1].Parentela[0].Quantita != 2 {
			t.Error("Figli restituisce la memoria dell'esito")
		}
	}
}

// Legami dopo uno «Sposta»: 11 è nel file sotto #1 e confermato sotto 10 (il componente della radice della BOM di
// lavoro, T-A1d-22) e sotto 15, che il file non ha (X-05; D5 di P7e). Nella scheda «senza» il padre di 12 si chiama
// come il Padre della sua Parentela (la regola 2 di rifNelGruppo). Per ogni arco di ogni gruppo, Figli[].Decisa e
// Legami.Confermati[].NelFile dicono la stessa cosa (D-PF-02; RAC-09).
func TestIndiceMotoreALegamiDopoUnoSpostamento(t *testing.T) {
	x, _, _ := maIndiceACME()
	l := x.Legami(maP1, "bom", maNodo(1, "#2"))
	if !reflect.DeepEqual(l.DelFile, []LegameFile{{Padre: maNodo(1, "#1"), Quantita: maInt(2), QuantitaConfermata: maInt(2), Decisa: true}}) {
		t.Errorf("Legami(bom, #2).DelFile = %+v", l.DelFile)
	}
	attesi := []LegameConfermato{
		{PadreID: maUUID(10), PadreRif: maNodo(1, "#1"), Codice: "7120001", Qta: 2, NelFile: true},
		{PadreID: maUUID(15), PadreRif: maComp(15), Codice: "7120099", Qta: 1, NelFile: false},
	}
	if !reflect.DeepEqual(l.Confermati, attesi) {
		t.Errorf("Legami(bom, #2).Confermati = %+v, attesi %+v", l.Confermati, attesi)
	}
	l = x.Legami(maP1, "senza", maComp(12))
	if l.DelFile != nil || !reflect.DeepEqual(l.Confermati, []LegameConfermato{
		{PadreID: maUUID(11), PadreRif: maNodo(1, "#2"), Codice: "7120010", Qta: 4, NelFile: false}}) {
		t.Errorf("Legami(senza, 12) = %+v", l)
	}
	// Un nodo dello STEP senza un file ancorato (#3, senza componente): i suoi padri del file con la quantità dei fatti,
	// nessuna relazione confermata.
	l = x.Legami(maP1, "bom", maNodo(1, "#3"))
	if len(l.DelFile) != 2 || maQ(l.DelFile[0].Quantita) != "8" || maQ(l.DelFile[1].Quantita) != "1" || l.Confermati != nil {
		t.Errorf("Legami(bom, #3) = %+v", l)
	}
	// La radice di SC3 ha lo stesso Rif della radice della BOM, ma in una candidata non prende il componente del target.
	if l := x.Legami(maP1, maChiaveCand(32, maNodo(1, "#1")), maNodo(1, "#1")); l.Confermati != nil {
		t.Errorf("la radice di una candidata ha legami confermati: %+v", l.Confermati)
	}
	if l := x.Legami(maP1, "bom", maNodo(9, "#9")); l.DelFile != nil || l.Confermati != nil {
		t.Error("Legami di un nodo che non c'è")
	}
	// RAC-09: per lo stesso arco, il tratto (Figli[].Decisa) e il testo (Confermati[].NelFile) coincidono; per un
	// confermato senza STEP l'arco è la relazione confermata stessa (Decisa vero, NelFile falso perché non c'è un file).
	for _, g := range x.Gruppi(maP1) {
		for _, n := range g.Nodi {
			l := x.Legami(maP1, g.Chiave, n.Nodo.Rif)
			for _, par := range n.Parentela {
				var decisa *bool
				for _, f := range x.Figli(maP1, g.Chiave, par.Padre) {
					if f.Nodo == n {
						d := f.Decisa
						decisa = &d
					}
				}
				if decisa == nil {
					if x.NodoIn(maP1, g.Chiave, par.Padre) != nil {
						t.Errorf("%s: %s non è fra i figli di %s", g.Chiave, n.Nodo.Rif, par.Padre)
					}
					continue
				}
				confermato := false
				for _, c := range l.Confermati {
					if c.PadreRif == par.Padre && (c.NelFile || confermatoSenzaSTEP(n)) {
						confermato = true
					}
				}
				if *decisa != confermato {
					t.Errorf("%s: l'arco %s → %s ha Decisa=%v e un legame confermato=%v", g.Chiave, par.Padre, n.Nodo.Rif, *decisa, confermato)
				}
			}
		}
	}
}

// La BOM di lavoro si cerca per stato, non per posizione (bomDi; R-5 della revisione di A1d-P2): con una struttura
// candidata dello stesso prodotto PRIMA della BOM in Ancoraggi.Strutture, il gruppo «bom» ha ancora lo STEP, la radice e
// il nome della BOM (allegato 31), e la radice della BOM prende ancora il componente del target (il legame confermato
// di #2 ha per padre il nodo #1 nel file). Le risposte sono quelle dell'ordine di sempre.
func TestIndiceMotoreALaBOMNonELaPrimaStruttura(t *testing.T) {
	f, e, c := scenaIndiceACME()
	st := e.Thread[0].Ancoraggi.Strutture
	if len(st) < 3 || st[0].Stato != ancoraggio.StatoBOMDiLavoroProposta || st[2].Stato != ancoraggio.StatoStrutturaCandidata ||
		st[2].Target != maP1 || st[2].Radice == st[0].Radice {
		t.Fatal("la scena ACME è cambiata: la prova va riletta")
	}
	resto := append(append([]ancoraggio.StrutturaProdotto{}, st[:2]...), st[3:]...)
	e.Thread[0].Ancoraggi.Strutture = append([]ancoraggio.StrutturaProdotto{st[2]}, resto...)
	x := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c)
	y, _, _ := maIndiceACME()

	g := x.Gruppi(maP1)
	if len(g) == 0 || g[0].Chiave != "bom" || g[0].AllegatoID == nil || *g[0].AllegatoID != maUUID(31) ||
		g[0].Radice != maNodo(1, "#1") || g[0].NomeFile != "7120001A_1.stp" {
		t.Fatalf("Gruppi(P1)[0] con la candidata prima della BOM = %+v, atteso il gruppo bom dell'allegato 31", g)
	}
	gy := y.Gruppi(maP1)
	if len(g) != len(gy) {
		t.Fatalf("%d gruppi, %d con l'ordine di sempre", len(g), len(gy))
	}
	for i := range g {
		if g[i].Chiave != gy[i].Chiave || g[i].Radice != gy[i].Radice || g[i].NomeFile != gy[i].NomeFile ||
			(g[i].AllegatoID == nil) != (gy[i].AllegatoID == nil) || (g[i].AllegatoID != nil && *g[i].AllegatoID != *gy[i].AllegatoID) {
			t.Errorf("gruppo %d: %q %q %q, con l'ordine di sempre %q %q %q", i, g[i].Chiave, g[i].Radice, g[i].NomeFile,
				gy[i].Chiave, gy[i].Radice, gy[i].NomeFile)
		}
	}
	l := x.Legami(maP1, "bom", maNodo(1, "#2"))
	if len(l.Confermati) == 0 || l.Confermati[0].PadreRif != maNodo(1, "#1") {
		t.Errorf("Legami(bom, #2).Confermati = %+v: il padre confermato è il nodo #1 della BOM", l.Confermati)
	}
	if ly := y.Legami(maP1, "bom", maNodo(1, "#2")); !reflect.DeepEqual(l, ly) {
		t.Errorf("Legami(bom, #2) = %+v, con l'ordine di sempre %+v", l, ly)
	}
}

// RelazioneProposta: la riga aperta fra gli allegati del prodotto, mai di un altro prodotto; nil se è in più allegati o
// non è aperta (contratto §3.4 B).
func TestIndiceMotoreARelazioneProposta(t *testing.T) {
	x, _, _ := maIndiceACME()
	for _, c := range []struct {
		prodotto, padre, figlio string
		attesa                  *RelazioneAperta
	}{
		{maP1, maNodo(1, "#1"), maNodo(1, "#2"), &RelazioneAperta{Allegato: maUUID(31), Padre: "#1", Figlio: "#2"}},
		{maP1, maNodo(1, "#1"), maNodo(1, "#3"), nil}, // in 31 e in 32, lo stesso contenuto: si decide nel Fascicolo
		{maP1, maNodo(1, "#1"), maNodo(1, "#4"), nil}, // la riga è di 37, un allegato di P2
		{maP2, maNodo(1, "#1"), maNodo(1, "#4"), &RelazioneAperta{Allegato: maUUID(37), Padre: "#1", Figlio: "#4"}},
		{maP1, maNodo(1, "#2"), maNodo(1, "#3"), nil}, // confermata, non aperta
		{maP1, maComp(10), maComp(11), nil},
		{"identificativo:7120003", maNodo(1, "#1"), maNodo(1, "#2"), nil},
	} {
		if got := x.RelazioneProposta(c.prodotto, c.padre, c.figlio); !reflect.DeepEqual(got, c.attesa) {
			t.Errorf("RelazioneProposta(%s, %s, %s) = %+v, attesa %+v", c.prodotto, c.padre, c.figlio, got, c.attesa)
		}
	}
}

// Rimozione: solo un conflitto d'arco con il motivo rimozione e il documento dello STEP, con la sua riga aperta.
func TestIndiceMotoreARimozione(t *testing.T) {
	x, _, et := maIndiceACME()
	cc := et.Conflitti
	if r := x.Rimozione(cc[0]); r == nil || *r != (RimozioneAperta{Step: maUUID(41), Padre: maUUID(10), Figlio: maUUID(11)}) {
		t.Errorf("Rimozione(c1) = %+v", r)
	}
	for i, nome := range []string{"", "c2 (quantità, con il documento dello step)", "c3 (un altro step)", "c4 (non un arco)", "c5 (nuovo_file)"} {
		if i == 0 {
			continue
		}
		if r := x.Rimozione(cc[i]); r != nil {
			t.Errorf("Rimozione di %s = %+v", nome, r)
		}
	}
	senzaDocumento := cc[0]
	senzaDocumento.EvidenzaProposta.DocumentoID = nil
	altroArco := cc[0]
	altroArco.Rif = valutazione.RifArco(maComp(10), maComp(12))
	if x.Rimozione(senzaDocumento) != nil || x.Rimozione(altroArco) != nil {
		t.Error("una rimozione senza documento, o di un altro arco, non ha una riga")
	}
}

// AutorizzazioneDi e AutorizzazioneDiStruttura (contratto §3.4 A; T-A1d-23): modo documento per una radice del file,
// con il documento confermato del componente con quell'allegato; modo delega per una radice interna; due strutture
// nello stesso allegato → nessuna Sorgente.
func TestIndiceMotoreAAutorizzazioneDi(t *testing.T) {
	x, _, _ := maIndiceACME()
	for _, c := range []struct {
		nome   string
		got    *Autorizzazione
		attesa *Autorizzazione
	}{
		{"la BOM di 31, radice del file", x.AutorizzazioneDiStruttura(maP1, maUUID(31), maNodo(1, "#1")), &Autorizzazione{Documento: maUUIDp(41), Sorgente: "#1"}},
		{"AutorizzazioneDi(P1, 31): una struttura", x.AutorizzazioneDi(maP1, maUUID(31)), &Autorizzazione{Documento: maUUIDp(41), Sorgente: "#1"}},
		{"SC2, radice interna: la delega", x.AutorizzazioneDiStruttura(maP1, maUUID(33), maNodo(2, "#2")),
			&Autorizzazione{Nodo: maUUID(33).String() + ":#2"}},
		{"SC1, radice del file senza documento di 10 con 33", x.AutorizzazioneDiStruttura(maP1, maUUID(33), maNodo(2, "#1")), nil},
		{"AutorizzazioneDi(P1, 33): due strutture, nessun documento", x.AutorizzazioneDi(maP1, maUUID(33)), nil},
		{"una radice che in quell'allegato non c'è", x.AutorizzazioneDiStruttura(maP1, maUUID(31), maNodo(2, "#1")), nil},
		{"un allegato senza strutture", x.AutorizzazioneDi(maP1, maUUID(35)), nil},
		{"P2 senza documento di 20 con 37", x.AutorizzazioneDi(maP2, maUUID(37)), nil},
		{"un prodotto che non c'è", x.AutorizzazioneDi("identificativo:7120003", maUUID(31)), nil},
	} {
		if !reflect.DeepEqual(c.got, c.attesa) {
			t.Errorf("%s: %+v, attesa %+v", c.nome, c.got, c.attesa)
		}
	}
	// Un documento corrente di 10 anche con 33: SC1 va nel modo documento, e AutorizzazioneDi(P1, 33), con due
	// strutture, dà il documento senza Sorgente (l'anteprima elenca le radici, U7). La chiave viene dalla riga di
	// componente_proposta; senza la riga la risposta è nil: il Rif non si spezza (R10, T-A1d-FE-01; la PR #18 ricavava
	// la chiave dal Rif).
	f, e, c := scenaIndiceACME()
	th := &f.Thread[0]
	th.Documenti[0].Allegati = append(th.Documenti[0].Allegati, maUUID(33))
	y := nuovoIndice3(th, &e.Thread[0], &c)
	if a := y.AutorizzazioneDiStruttura(maP1, maUUID(33), maNodo(2, "#1")); !reflect.DeepEqual(a, &Autorizzazione{Documento: maUUIDp(41), Sorgente: "#1"}) {
		t.Errorf("SC1 con il documento: %+v", a)
	}
	if a := y.AutorizzazioneDi(maP1, maUUID(33)); !reflect.DeepEqual(a, &Autorizzazione{Documento: maUUIDp(41)}) {
		t.Errorf("due strutture con il documento: %+v", a)
	}
	if a := y.AutorizzazioneDiStruttura(maP1, maUUID(33), maNodo(2, "#2")); !reflect.DeepEqual(a, &Autorizzazione{Nodo: maUUID(33).String() + ":#2"}) {
		t.Errorf("la delega con la riga: %+v", a)
	}
	th.RigheComponenteProposta = th.RigheComponenteProposta[:2]
	y = nuovoIndice3(th, &e.Thread[0], &c)
	if a := y.AutorizzazioneDiStruttura(maP1, maUUID(33), maNodo(2, "#1")); a != nil {
		t.Errorf("SC1 senza la riga: %+v, attesa nil (il Rif non si spezza)", a)
	}
	if a := y.AutorizzazioneDiStruttura(maP1, maUUID(33), maNodo(2, "#2")); a != nil {
		t.Errorf("la delega senza la riga: %+v, attesa nil (il Rif non si spezza)", a)
	}
	// Un documento sostituito non vale; un prodotto senza componente non ha l'autorizzazione (la rotta vuole {cid}).
	f, e, c = scenaIndiceACME()
	f.Thread[0].Documenti[0].SostituitoDa = maUUIDp(42)
	if a := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c).AutorizzazioneDi(maP1, maUUID(31)); a != nil {
		t.Errorf("con il documento sostituito: %+v", a)
	}
	f, e, c = scenaIndiceACME()
	e.Thread[0].ProdottiValutati[0].ComponenteID = nil
	z := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c)
	if z.AutorizzazioneDiStruttura(maP1, maUUID(31), maNodo(1, "#1")) != nil || z.AutorizzazioneDi(maP1, maUUID(31)) != nil ||
		z.AutorizzazioneDiStruttura(maP1, maUUID(33), maNodo(2, "#2")) != nil {
		t.Error("un prodotto senza componente non ha l'autorizzazione")
	}
}

// Le altre ricerche: Componente, Correnti, Voce, CopieAperte, CandidatiFuori, Conflitti per asse, ConflittiDelThread,
// Anteprima (X-09; T-A1d-19, T-A1d-24, T-A1d-25, T-A1d-26).
func TestIndiceMotoreALeAltreRicerche(t *testing.T) {
	x, th, et := maIndiceACME()
	p1 := &et.ProdottiValutati[0]

	// Componente: con uuid.UUID, *uuid.UUID, «componente:<uuid>»; il resto nil.
	id11 := maUUID(11)
	for _, v := range []any{maUUID(11), &id11, maComp(11)} {
		c := x.Componente(v)
		if c == nil || c.ID != id11 || c.Codice != "7120010" || c.Rev == nil || *c.Rev != "B" || c.Descrizione != nil ||
			c.Tipo != "sottoassieme" || c.Archiviato || c.StepStrutturale != nil {
			t.Errorf("Componente(%T) = %+v", v, c)
		}
	}
	if c := x.Componente(maUUID(13)); c == nil || !c.Archiviato || c.Tipo != "commerciale" {
		t.Errorf("Componente(13) = %+v", c)
	}
	if c := x.Componente(maComp(10)); c == nil || c.Descrizione == nil || *c.Descrizione != "Gruppo ACME" || c.StepStrutturale == nil ||
		*c.StepStrutturale != maUUID(41) {
		t.Errorf("Componente(10) = %+v", c)
	} else {
		*c.Descrizione, *c.StepStrutturale = "cambiata", uuid.Nil
		if *th.Componenti[0].Descrizione != "Gruppo ACME" || *th.Componenti[0].StepStrutturaleID != maUUID(41) {
			t.Error("Componente restituisce la memoria della fotografia")
		}
	}
	for _, v := range []any{nil, (*uuid.UUID)(nil), "7120010", "componente:xyz", "componente:" + strings.ReplaceAll(maUUID(11).String(), "-", ""),
		"componente:{" + maUUID(11).String() + "}",
		" " + maComp(11), maComp(99), 11, maNodo(1, "#2")} {
		if c := x.Componente(v); c != nil {
			t.Errorf("Componente(%#v) = %+v, atteso nil", v, c)
		}
	}

	// Correnti: i documenti correnti del componente con quel tipo, nell'ordine della fotografia.
	if c := x.Correnti(maUUID(11), "disegno_2d"); !reflect.DeepEqual(c, []DocumentoCorrente{{DocumentoID: maUUID(43), NomeFile: "7120010_B.pdf", Rev: maStr("B")}}) {
		t.Errorf("Correnti(11, disegno_2d) = %+v", c)
	}
	if c := x.Correnti(maUUID(10), "cad_3d"); len(c) != 1 || c[0].DocumentoID != maUUID(41) || c[0].Rev != nil {
		t.Errorf("Correnti(10, cad_3d) = %+v", c)
	}
	if x.Correnti(maUUID(11), "cad_3d") != nil || x.Correnti(maUUID(13), "disegno_2d") != nil {
		t.Error("Correnti di un tipo o di un componente senza documenti")
	}

	// Voce e CopieAperte (StatoNas in coda o in errore, non scritto).
	if x.Voce(maP1, maUUID(12), "cad_3d") != &p1.Documenti.Voci[1] || x.Voce(maP1, maUUID(12), "disegno_2d") != nil || x.Voce(maP2, maUUID(12), "cad_3d") != nil {
		t.Error("Voce non trova la voce per (componente, tipo)")
	}
	if c := x.CopieAperte(maP1); len(c) != 2 || c[0] != &p1.Documenti.Voci[0] || c[1] != &p1.Documenti.Voci[1] {
		t.Errorf("CopieAperte(P1) = %d voci", len(c))
	}
	if x.CopieAperte(maP2) != nil {
		t.Error("CopieAperte(P2)")
	}

	// CandidatiFuori: (33) è l'allegato di SC1 e SC2, (31) ha lo sha di SC3 (32, lo stesso contenuto): resta (36), copiato.
	if c := x.CandidatiFuori(maP1); len(c) != 1 || c[0].AllegatoID == nil || *c[0].AllegatoID != maUUID(36) {
		t.Errorf("CandidatiFuori(P1) = %+v", c)
	} else {
		*c[0].AllegatoID, c[0].Radici[0] = uuid.Nil, "cambiata"
		if *p1.Fonte.Candidati[2].AllegatoID != maUUID(36) || p1.Fonte.Candidati[2].Radici[0] != "#9" {
			t.Error("CandidatiFuori restituisce la memoria dell'esito")
		}
	}

	// Conflitti per asse (T-A1d-25) e del thread.
	cc := et.Conflitti
	for _, c := range []struct {
		prodotto, asse string
		attesi         []valutazione.Conflitto
	}{
		{maP1, "", cc[:4]},
		{maP1, "struttura", cc[:3]},
		{maP1, "gerarchia", cc[:3]},
		{maP1, "smistamento", cc[3:4]},
		{maP1, "nomenclatura", nil},
		{maP1, "boh", nil},
		{maP2, "", nil},
	} {
		if got := x.Conflitti(c.prodotto, c.asse); !reflect.DeepEqual(got, c.attesi) {
			t.Errorf("Conflitti(%s, %q): %d conflitti, attesi %d", c.prodotto, c.asse, len(got), len(c.attesi))
		}
	}
	if got := x.ConflittiDelThread(); !reflect.DeepEqual(got, cc[4:]) {
		t.Errorf("ConflittiDelThread() = %+v", got)
	}

	// Anteprima: il 2D dell'esito (una copia); altrimenti dall'allegato, con il formato e mai disponibile (IX-07 della
	// revisione di A1d-P2: se il PDF si apre lo dice la GET dell'anteprima); senza sha nil.
	if a := x.Anteprima(maUUID(34)); !reflect.DeepEqual(a, &p1.Documenti.Voci[0].Disegni.Primario.Anteprima) {
		t.Errorf("Anteprima(34) = %+v", a)
	} else {
		*a.AllegatoID = uuid.Nil
		if *p1.Documenti.Voci[0].Disegni.Primario.Anteprima.AllegatoID != maUUID(34) || *x.Anteprima(maUUID(34)).AllegatoID != maUUID(34) {
			t.Error("Anteprima restituisce la memoria dell'esito")
		}
	}
	if a := x.Anteprima(maUUID(35)); !reflect.DeepEqual(a, &valutazione.RiferimentoAnteprima{Sha256: maSha(4), Formato: valutazione.Formato2DPDF,
		AllegatoID: maUUIDp(35)}) {
		t.Errorf("Anteprima(35) = %+v", a)
	}
	if a := x.Anteprima(maUUID(36)); !reflect.DeepEqual(a, &valutazione.RiferimentoAnteprima{Sha256: maSha(5), AllegatoID: maUUIDp(36)}) {
		t.Errorf("Anteprima(36) = %+v", a)
	}
	if x.Anteprima(maUUID(38)) != nil || x.Anteprima(maUUID(99)) != nil {
		t.Error("Anteprima senza sha256, o di un allegato che non c'è")
	}
	// L'estensione dichiarata vale prima di quella del nome.
	f, e, c := scenaIndiceACME()
	f.Thread[0].Allegati[5].Estensione = maStr(".PDF")
	if a := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c).Anteprima(maUUID(36)); a == nil || a.Formato != valutazione.Formato2DPDF || a.Disponibile {
		t.Errorf("Anteprima(36) con l'estensione dichiarata: %+v", a)
	}
}
