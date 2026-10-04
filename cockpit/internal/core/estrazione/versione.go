// Package estrazione contiene gli adattatori del motore A: dai record della fotografia (messaggio, allegato,
// fatti del worker) ai DocumentoEvidenze (parte 1 §2, §3, §7.2; piano A, par.3.3.5 e 5.4.5).
//   - Nessuna regola cliente è applicata ai testi: il pacchetto non importa grammatica né motorea (G1).
//   - Nessun parser nuovo: dove il dato manca, la capacità è dichiarata non disponibile, con il motivo (parte 1
//     §13.4). Un file senza fatti non è un file «senza codici».
//   - Nessuna relazione indovinata: due cose si collegano solo quando i fatti lo dicono (stessa entità, stesso
//     nodo STEP, relazione dichiarata dal worker), mai per vicinanza o somiglianza (5.0).
//   - Legge i fatti del worker fuori da classificazione. È un cambio deliberato della convenzione di
//     classificazione/testo_pdf.go:19-22, scritto nel README; il percorso esistente non cambia (5.10 n.11).
//   - Puro: niente orologio, file, DB, rete, goroutine; gli ID locali nascono da chiavi del contenuto (G2).
package estrazione

// VersioneAdattatore: regole di mappatura, segmentazione e identità locali degli adattatori, più la versione di
// golang.org/x/net (v0.58.0, go.mod) da cui dipendono i percorsi nel DOM delle tabelle della mail (5.4.5,
// 5.6.1). Comprende le mappature qui sotto e la regola legami-1 (5.4.1): un'appartenenza sta in un campo solo,
// e i LegameFonte nascono solo per ciò che non è già un campo. Entra nel BundleID di ogni documento. Si cambia
// solo con un commit che lo dichiara, e la prova che la fissa si riscrive con «Riscritta per …» (par.3.4.1).
const VersioneAdattatore = "adattatori-1"

// Le mappature che VersioneAdattatore comprende. Ognuna si scrive in CampoOriginale.Mappatura delle unità che
// produce: chi legge un'unità sa con quale regola è nata.
const (
	mappaturaSTEP  = "mappatura-step-1"  // i fatti della struttura STEP
	mappaturaPDF   = "mappatura-pdf-1"   // i fatti del testo dei PDF
	mappaturaNome  = "mappatura-nome-1"  // il nome del file e la voce d'archivio
	mappaturaEmail = "mappatura-email-1" // oggetto, segmenti e tabelle della mail
)

// maxLivelliStoria: quanti livelli della storia l'adattatore della mail separa al più (5.4.3 punto 6; 5.6.1).
// Oltre, il resto resta nell'ultimo livello, con email.livelli_oltre_limite. Non è uno dei limiti dell'indice
// delle regole (R43 B): gli adattatori non leggono l'indice, quindi il valore sta nella versione dell'adattatore.
const maxLivelliStoria = 8
