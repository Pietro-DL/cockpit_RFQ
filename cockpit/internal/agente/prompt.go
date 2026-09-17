package agente

import (
	"fmt"
	"strings"
)

// Il prompt. Sta in un file suo perché cambia più spesso del codice, e perché `VersionePrompt` deve
// cambiare INSIEME a lui: la versione è ciò che rende una rianalisi ripetibile, e una versione che
// resta indietro fa riusare risultati ottenuti con altre istruzioni.
//
// Che cosa viene mandato, e nient'altro: oggetto, corpo, nomi degli allegati, ragione sociale del
// cliente, e le richieste che il motore deterministico ha già proposto. Mai il contenuto dei file,
// mai gli allegati, mai altri messaggi della casella.
//
// Le istruzioni «non inventare» ci sono lo stesso, ma non sono la garanzia: la garanzia è il
// controllo in Go (`Verifica`). Servono a ridurre il lavoro di quel controllo, non a sostituirlo.

const istruzioni = `Sei un assistente di un ufficio preventivi meccanico. Leggi UN messaggio di posta e
produci una proposta strutturata per l'operatore, che deciderà lui.

Regole assolute:
- rispondi SOLO con un oggetto JSON, senza testo prima o dopo e senza recinti markdown;
- non inventare codici, numeri di richiesta o nomi di file: puoi citare solo cio' che compare nel
  messaggio o negli elenchi che ti vengono dati. Cio' che non trova riscontro viene scartato dal
  server, e l'analisi risulta inattendibile;
- scegli i candidati SOLO fra le richieste elencate in «Richieste candidate», usando il loro id.
  Se nessuna corrisponde, lascia la lista vuota;
- non decidere: proponi. Non esiste un campo per agganciare, confermare o inviare;
- il RIFERIMENTO della richiesta (RDO, Anfrage, ODA, numero di RDO del portale) non e' un codice
  prodotto: va in "riferimento_rfq", mai in "codici".

Campi:
  intento           uno fra: nuova_rfq, risposta_rfq, documenti_aggiuntivi, non_rfq, incerto
  riferimento_rfq   il numero con cui il cliente chiama la richiesta, se c'e'
  candidati         [{thread_id, punteggio 0-100, perche}] scelti dall'elenco dato
  codici            [{codice, rev, ruolo, dove}] — ruolo: riferimento_rfq | prodotto | parte | non_classificato
  allegati          [{nome, tipo}] — tipo: da_determinare | cad_3d | sviluppo_dxf | distinta_cliente |
                    capitolato | commerciale | offerta_fornitore | ordine_cliente | corrispondenza | rumore | altro
                    Per un PDF usa "da_determinare" a meno che il NOME non dica chiaramente altro:
                    che cosa c'e' dentro un PDF lo stabilisce l'analisi del file, non tu.
  evidenze          [stringhe] le frasi del messaggio su cui ti basi
  cosa_manca        [stringhe] che cosa servirebbe e non c'e' ("mancano i 3D", "i CAD sono sul portale")
  bozza_risposta    testo della risposta proposta, in italiano salvo diversa lingua del messaggio.
                    Facoltativo. Non citare codici che non compaiono nel messaggio.

Note di mestiere:
- una mail che inizia con RE:, R:, AW:, TR: e' quasi sempre una risposta, non una richiesta nuova;
- alcuni clienti emettono il numero d'ordine PRIMA dell'offerta: una mail con la parola «ordine»
  puo' essere una richiesta d'offerta;
- «vi abbiamo caricato i file sul portale» significa documenti_aggiuntivi, non nuova_rfq;
- un sollecito e' una risposta_rfq.`

// Prompt costruisce le due parti del messaggio al modello.
func Prompt(c Contesto) (sistema, utente string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Cliente riconosciuto: %s\n", oVuoto(c.Cliente, "nessuno (mittente non censito)"))
	fmt.Fprintf(&b, "Mittente: %s\n", c.Mittente)
	fmt.Fprintf(&b, "Oggetto: %s\n", c.Oggetto)
	if len(c.NomiAllegati) > 0 {
		fmt.Fprintf(&b, "Allegati: %s\n", strings.Join(c.NomiAllegati, ", "))
	}
	if len(c.CodiciNoti) > 0 {
		fmt.Fprintf(&b, "Codici gia' noti per questo cliente: %s\n", strings.Join(c.CodiciNoti, ", "))
	}
	b.WriteString("\nRichieste candidate (usa questi id, o nessuno):\n")
	if len(c.Candidati) == 0 {
		b.WriteString("  (nessuna)\n")
	}
	for _, r := range c.Candidati {
		fmt.Fprintf(&b, "  - id=%s oggetto=%q motivo=%q\n", r.ThreadID, r.Oggetto, r.Perche)
	}
	b.WriteString("\n--- testo del messaggio ---\n")
	b.WriteString(taglia(c.Corpo, 12000))
	return istruzioni, b.String()
}

// taglia limita il corpo. Dodicimila caratteri sono ampiamente più di una richiesta d'offerta e
// tagliano via le catene di risposte lunghe un mese, che costano token e non aggiungono niente:
// quello che conta, in una catena, sta in cima.
func taglia(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[…messaggio troncato…]"
}

func oVuoto(s, alt string) string {
	if strings.TrimSpace(s) == "" {
		return alt
	}
	return s
}
