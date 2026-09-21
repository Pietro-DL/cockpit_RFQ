package domain

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// LA CONTROPARTE DI UN MESSAGGIO (blocco 7A, D33)
//
// Fino a qui il Cockpit conosceva una sola specie di interlocutore, il cliente, e chiunque scrivesse
// da un dominio non censito era «mittente non censito». Una richiesta d'offerta di un FORNITORE —
// «RICHIESTA D'OFFERTA ... TG FIORE», con il PDF della nostra richiesta allegato — ha le stesse
// parole e gli stessi allegati di una RFQ del cliente, e il triage la proponeva come RFQ nuova.
//
// La controparte e' un FATTO sul messaggio: chi c'e' dall'altra parte, e da che cosa lo si e'
// capito. Si risolve dall'anagrafica con una precedenza fissa, e il risultato si scrive sul
// messaggio con la via e l'ora, cosi' che si possa sempre dire «perche' questo messaggio e' di un
// fornitore» guardando la riga.
//
// # La precedenza
//
//  1. il mittente e' NOSTRO (casella censita o dominio nostro): si guarda il primo destinatario
//     esterno, con la stessa scala; se non ce n'e', e' traffico `interno`;
//  2. l'indirizzo ESATTO e' un contatto di un fornitore, un buyer di un cliente, oppure un recapito
//     di un soggetto «altro» (7C.0). In piu' d'una di queste → `ambiguo`; in due fornitori diversi
//     → `ambiguo`;
//  3. il DOMINIO e' di un fornitore, di un cliente, oppure di un soggetto «altro». In piu' d'una →
//     `ambiguo`;
//  4. altrimenti `sconosciuto`.
//
// `ambiguo` non e' un errore di anagrafica: un gruppo che compra e vende puo' avere lo stesso
// dominio nelle due liste. E' una risposta, e vuol dire «decide una persona». A parita' di
// specificita' non c'e' MAI una precedenza silenziosa fra le categorie: due che riconoscono lo
// stesso recapito danno `ambiguo`. L'indirizzo esatto vince sul dominio, e questo basta perche'
// «newsletter@cliente.example» censito come Altro non finisca fra i clienti.
//
// `altro` e' sempre censito da una persona: uno sconosciuto resta sconosciuto finche' qualcuno
// non decide, e nessun automatismo (tanto meno l'agente) lo sposta qui.
//
// Il pacchetto non sa niente del database: chi chiama passa una Rubrica. Cosi' la tabella dei casi
// (CP6) si prova con una rubrica in memoria, e l'ingest passa quella vera.

// I tipi e le vie hanno le stesse stringhe degli enum di PostgreSQL (`tipo_controparte`,
// `via_controparte`): un valore inventato qui non entrerebbe in database.
const (
	ControparteCliente     = "cliente"
	ControparteFornitore   = "fornitore"
	ControparteInterno     = "interno"
	ControparteAltro       = "altro" // un soggetto censito come diverso da cliente, fornitore e interno (0016)
	ControparteSconosciuto = "sconosciuto"
	ControparteAmbiguo     = "ambiguo"

	ViaContatto = "contatto" // email esatta in contatto_fornitore, in buyer o in recapito_altro
	ViaDominio  = "dominio"  // dominio in dominio_fornitore, in dominio_cliente o in recapito_altro
	ViaCasella  = "casella"  // mittente e destinatari sono nostri
	ViaManuale  = "manuale"  // deciso da un operatore
)

// Voce e' una riga di anagrafica trovata dalla rubrica: chi e', e come si chiama per l'evidenza.
type Voce struct {
	ID   uuid.UUID
	Nome string
}

// Rubrica sono le cinque domande che il resolver fa all'anagrafica. Ogni metodo risponde «trovato
// o no»: un errore e' un errore del database, e ferma la risoluzione invece di farla passare per
// `sconosciuto`.
type Rubrica interface {
	// ContattiFornitore: TUTTI i fornitori che hanno questa email fra i contatti (la chiave unica
	// e' per fornitore, quindi possono essere due).
	ContattiFornitore(ctx context.Context, email string) ([]Voce, error)
	BuyerCliente(ctx context.Context, email string) (Voce, bool, error)
	FornitorePerDominio(ctx context.Context, dominio string) (Voce, bool, error)
	ClientePerDominio(ctx context.Context, dominio string) (Voce, bool, error)
	// AltroPerRecapito: il soggetto «altro» che ha questo recapito, che sia un indirizzo o un
	// dominio (una domanda sola: la tabella e' una).
	AltroPerRecapito(ctx context.Context, recapito string) (Voce, bool, error)
}

// IngressoControparte e' cio' che serve per decidere: da chi viene, a chi va, e chi siamo noi.
type IngressoControparte struct {
	Mittente    string
	Destinatari []string
	// Nostro dice se un indirizzo e' una casella censita o un dominio nostro. Nil = non lo sappiamo:
	// si risolve il mittente e basta.
	Nostro func(indirizzo string) bool
}

// Controparte e' la risposta.
type Controparte struct {
	Tipo string // uno dei Controparte*
	Via  string // uno dei Via*; vuota per `sconosciuto`
	// ClienteID, FornitoreID o AltroID: uno solo dei tre e' valorizzato, e solo per `cliente`,
	// `fornitore` e `altro`.
	ClienteID   uuid.UUID
	FornitoreID uuid.UUID
	AltroID     uuid.UUID
	// Indirizzo e' quello su cui si e' deciso: il mittente, oppure il primo destinatario esterno
	// di una mail nostra. In minuscolo.
	Indirizzo string
	// Nome e' la ragione sociale (o la cartella) di chi e' stato riconosciuto, per la UI e il log.
	Nome string
	// Motivo e' la frase che spiega la decisione: finisce nei motivi del triage e nel log del
	// ricalcolo.
	Motivo string
}

// RisolviControparte applica la precedenza. Non scrive niente.
func RisolviControparte(ctx context.Context, in IngressoControparte, r Rubrica) (Controparte, error) {
	mittente := normalizzaIndirizzo(in.Mittente)
	if in.Nostro != nil && mittente != "" && in.Nostro(mittente) {
		for _, d := range in.Destinatari {
			d = normalizzaIndirizzo(d)
			if d == "" || in.Nostro(d) {
				continue
			}
			c, err := risolviIndirizzo(ctx, d, r)
			if err != nil {
				return Controparte{}, err
			}
			return c, nil
		}
		return Controparte{Tipo: ControparteInterno, Via: ViaCasella, Indirizzo: mittente,
			Motivo: "mittente e destinatari sono nostri"}, nil
	}
	return risolviIndirizzo(ctx, mittente, r)
}

// risolviIndirizzo e' la scala per un indirizzo solo: contatto esatto, poi dominio, poi niente.
func risolviIndirizzo(ctx context.Context, indirizzo string, r Rubrica) (Controparte, error) {
	out := Controparte{Tipo: ControparteSconosciuto, Indirizzo: indirizzo}
	i := strings.LastIndex(indirizzo, "@")
	if i <= 0 || i == len(indirizzo)-1 {
		out.Motivo = "nessun indirizzo su cui decidere"
		return out, nil
	}
	fornitori, err := r.ContattiFornitore(ctx, indirizzo)
	if err != nil {
		return out, fmt.Errorf("contatti fornitore: %w", err)
	}
	buyer, eBuyer, err := r.BuyerCliente(ctx, indirizzo)
	if err != nil {
		return out, fmt.Errorf("buyer: %w", err)
	}
	altro, eAltro, err := r.AltroPerRecapito(ctx, indirizzo)
	if err != nil {
		return out, fmt.Errorf("recapito altro: %w", err)
	}
	// quante categorie riconoscono l'indirizzo esatto: piu' d'una e' `ambiguo`, senza precedenze
	categorie := 0
	for _, si := range []bool{len(fornitori) > 0, eBuyer, eAltro} {
		if si {
			categorie++
		}
	}
	switch {
	case categorie > 1:
		out.Tipo, out.Via = ControparteAmbiguo, ViaContatto
		out.Motivo = fmt.Sprintf("%s è censito in più anagrafiche (%s)", indirizzo, elencoCategorie(fornitori, eBuyer, buyer, eAltro, altro))
		return out, nil
	case len(fornitori) > 1:
		out.Tipo, out.Via = ControparteAmbiguo, ViaContatto
		out.Motivo = fmt.Sprintf("%s è contatto di %d fornitori (%s, %s)", indirizzo, len(fornitori), fornitori[0].Nome, fornitori[1].Nome)
		return out, nil
	case len(fornitori) == 1:
		out.Tipo, out.Via, out.FornitoreID, out.Nome = ControparteFornitore, ViaContatto, fornitori[0].ID, fornitori[0].Nome
		out.Motivo = fmt.Sprintf("%s è un contatto censito di %s (fornitore)", indirizzo, fornitori[0].Nome)
		return out, nil
	case eBuyer:
		out.Tipo, out.Via, out.ClienteID, out.Nome = ControparteCliente, ViaContatto, buyer.ID, buyer.Nome
		out.Motivo = fmt.Sprintf("%s è un buyer censito di %s", indirizzo, buyer.Nome)
		return out, nil
	case eAltro:
		out.Tipo, out.Via, out.AltroID, out.Nome = ControparteAltro, ViaContatto, altro.ID, altro.Nome
		out.Motivo = fmt.Sprintf("%s è un recapito censito di «%s» (altro)", indirizzo, altro.Nome)
		return out, nil
	}
	dominio := indirizzo[i+1:]
	f, eF, err := r.FornitorePerDominio(ctx, dominio)
	if err != nil {
		return out, fmt.Errorf("dominio fornitore: %w", err)
	}
	c, eC, err := r.ClientePerDominio(ctx, dominio)
	if err != nil {
		return out, fmt.Errorf("dominio cliente: %w", err)
	}
	a, eA, err := r.AltroPerRecapito(ctx, dominio)
	if err != nil {
		return out, fmt.Errorf("dominio altro: %w", err)
	}
	categorie = 0
	for _, si := range []bool{eF, eC, eA} {
		if si {
			categorie++
		}
	}
	switch {
	case categorie > 1:
		out.Tipo, out.Via = ControparteAmbiguo, ViaDominio
		out.Motivo = fmt.Sprintf("il dominio %s è censito in più anagrafiche (%s)", dominio, elencoCategorie(dominioVoci(eF, f), eC, c, eA, a))
	case eF:
		out.Tipo, out.Via, out.FornitoreID, out.Nome = ControparteFornitore, ViaDominio, f.ID, f.Nome
		out.Motivo = fmt.Sprintf("il dominio %s è censito per il fornitore %s", dominio, f.Nome)
	case eC:
		out.Tipo, out.Via, out.ClienteID, out.Nome = ControparteCliente, ViaDominio, c.ID, c.Nome
		out.Motivo = fmt.Sprintf("il dominio %s è censito per il cliente %s", dominio, c.Nome)
	case eA:
		out.Tipo, out.Via, out.AltroID, out.Nome = ControparteAltro, ViaDominio, a.ID, a.Nome
		out.Motivo = fmt.Sprintf("il dominio %s è censito per «%s» (altro)", dominio, a.Nome)
	default:
		out.Motivo = fmt.Sprintf("né %s né il dominio %s sono censiti", indirizzo, dominio)
	}
	return out, nil
}

func normalizzaIndirizzo(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// elencoCategorie e' la frase del motivo quando un recapito sta in piu' anagrafiche: dice quali,
// con i nomi, perche' e' cio' che l'operatore legge per decidere.
func elencoCategorie(fornitori []Voce, eBuyer bool, buyer Voce, eAltro bool, altro Voce) string {
	var parti []string
	for _, f := range fornitori {
		parti = append(parti, "fornitore "+f.Nome)
	}
	if eBuyer {
		parti = append(parti, "cliente "+buyer.Nome)
	}
	if eAltro {
		parti = append(parti, "altro «"+altro.Nome+"»")
	}
	return strings.Join(parti, ", ")
}

func dominioVoci(si bool, v Voce) []Voce {
	if si {
		return []Voce{v}
	}
	return nil
}

// RubricaFissa e' una rubrica in memoria: serve alle prove e al banco di prova dell'Anagrafica.
type RubricaFissa struct {
	Contatti        map[string][]Voce // email → fornitori
	Buyer           map[string]Voce   // email → cliente
	DominiFornitore map[string]Voce
	DominiCliente   map[string]Voce
	Altro           map[string]Voce // recapito (email o dominio) → soggetto altro
}

func (r RubricaFissa) AltroPerRecapito(_ context.Context, recapito string) (Voce, bool, error) {
	v, ok := r.Altro[recapito]
	return v, ok, nil
}

func (r RubricaFissa) ContattiFornitore(_ context.Context, email string) ([]Voce, error) {
	return r.Contatti[email], nil
}
func (r RubricaFissa) BuyerCliente(_ context.Context, email string) (Voce, bool, error) {
	v, ok := r.Buyer[email]
	return v, ok, nil
}
func (r RubricaFissa) FornitorePerDominio(_ context.Context, d string) (Voce, bool, error) {
	v, ok := r.DominiFornitore[d]
	return v, ok, nil
}
func (r RubricaFissa) ClientePerDominio(_ context.Context, d string) (Voce, bool, error) {
	v, ok := r.DominiCliente[d]
	return v, ok, nil
}

// dominiPubblici sono i domini di posta che non identificano nessuno: un contatto su gmail dice chi
// e' la persona, non l'azienda. Chi censisce da un messaggio con uno di questi domini deve
// censire l'INDIRIZZO, non il dominio, altrimenti il primo altro utente di gmail diventerebbe
// quel fornitore. L'elenco e' volutamente corto e non pretende di essere completo: un dominio
// pubblico che manca qui si vede perche' un giorno un fornitore ne assorbe un altro, e allora si
// aggiunge.
var dominiPubblici = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "outlook.it": true, "hotmail.com": true,
	"hotmail.it": true, "live.com": true, "live.it": true, "msn.com": true, "yahoo.com": true, "yahoo.it": true,
	"libero.it": true, "virgilio.it": true, "alice.it": true, "tin.it": true, "tiscali.it": true,
	"fastwebnet.it": true, "icloud.com": true, "me.com": true, "aol.com": true, "protonmail.com": true,
	"proton.me": true, "pec.it": true, "legalmail.it": true, "arubapec.it": true, "postecert.it": true,
}

// DominioPubblico dice se un dominio di posta e' di un fornitore di caselle e non di un'azienda.
func DominioPubblico(dominio string) bool {
	return dominiPubblici[strings.ToLower(strings.TrimSpace(dominio))]
}
