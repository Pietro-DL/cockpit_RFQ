package web

import "promatec/cockpit/internal/platform/db"

// La navigazione (8.8, D29)
//
// Fino al blocco 2 le schermate stavano tutte in una barra orizzontale, e finché erano cinque
// funzionava. Non scala: ogni blocco da qui al 10 ne aggiunge almeno una (Anagrafica, Struttura,
// Smistamento, Fascicolo, e più avanti Fornitori e Attrezzature), e una testata che cresce in
// larghezza smette di essere una navigazione molto prima di finire lo spazio — diventa un elenco
// in cui si cerca.
//
// Da qui la forma è: una RAIL a sinistra per la navigazione, raggruppata in sezioni, e la testata
// riservata al CONTESTO OPERATIVO (stato delle caselle, postazione della sessione, utente). Sono
// due cose diverse e stavano insieme: «dove voglio andare» e «come sta il sistema adesso».
//
// # Le sezioni
//
// L'operatore vede Inbox e Richieste: il suo lavoro. L'amministratore vede quelle più una sezione
// *Admin* con Anagrafica, Postazioni, Coda job, Integrità NAS, Scarti. La sezione nuova si aggiunge qui e compare
// nella rail: è il punto della voce 8.8, e il motivo per cui questa funzione esiste invece di una
// lista scritta dentro il template.
//
// # Il Cruscotto non è più una voce (checkpoint 3R §1)
//
// «Richieste» e «Cruscotto» erano due elenchi della stessa cosa, con le stesse colonne, in due
// pagine diverse: l'unica differenza era l'ordinamento. Due tabelle globali quasi identiche non
// sono due funzioni, sono una funzione e un doppione — e il doppione invecchia da solo, perché
// nessuno si ricorda di aggiornare tutt'e due.
//
// Adesso *Richieste* è l'ELENCO delle RFQ, e aprirne una porta al suo cruscotto: la pagina di
// lavoro di quella richiesta, dove nei blocchi 5, 8 e 10 arriveranno Struttura, Smistamento e
// Fascicolo. «Cruscotto» resta come parola, ma vuol dire il cruscotto DI UNA RICHIESTA, non una
// tabella di tutte. `/cruscotto` reindirizza a `/richieste`: i segnalibri di chi lo aveva salvato
// continuano a funzionare.
//
// # Anagrafica è amministrativa (D29)
//
// Il mockup e D21 la mettevano fra le voci operative. La decisione è superata: l'anagrafica
// decide come il sistema riconosce i clienti di tutti — una regex cambiata lì cambia il triage di
// novecento messaggi — ed è quindi dell'amministratore, sotto `/admin/*` come le altre.
//
// # Nascondere non è autorizzare
//
// Questa funzione decide che cosa si VEDE. Che cosa si può APRIRE lo decide `soloAdmin` sulle
// rotte, e le due cose chiamano lo stesso predicato — `almeno` — proprio perché non possano
// allontanarsi: una voce che si vede e porta a un 403 è una presa in giro, una voce nascosta su
// una rotta aperta è un falso senso di sicurezza.
type voceNav struct {
	Etichetta string
	Href      string
	// Titolo è il titolo della vista che rende attiva questa voce. La corrispondenza è per
	// titolo e non per percorso perché il percorso di una schermata cambia (e allora la voce
	// resterebbe spenta senza che nessun test se ne accorga), mentre il titolo è ciò che l'utente
	// legge in cima alla pagina: se non corrisponde, si vede a occhio.
	Titolo string
}

type sezioneNav struct {
	Nome string // "" = navigazione principale, senza intestazione
	Voci []voceNav
}

// navPer costruisce la rail per questo utente. Un utente nil (pagina di login) non ha
// navigazione: non c'è niente che possa aprire.
func navPer(u *db.Utente) []sezioneNav {
	if u == nil {
		return nil
	}
	nav := []sezioneNav{{Voci: []voceNav{
		{Etichetta: "Inbox", Href: "/inbox", Titolo: "Inbox"},
		{Etichetta: "Richieste", Href: "/richieste", Titolo: "Richieste"},
	}}}
	if almeno(u, db.RuoloUtenteAdmin) {
		nav = append(nav, sezioneNav{Nome: "Admin", Voci: []voceNav{
			{Etichetta: "Anagrafica", Href: "/admin/anagrafica", Titolo: "Anagrafica"},
			{Etichetta: "Postazioni", Href: "/admin/postazioni", Titolo: "Postazioni"},
			{Etichetta: "Coda job", Href: "/admin/job", Titolo: "Coda job"},
			{Etichetta: "Integrità NAS", Href: "/admin/nas", Titolo: "Integrità NAS"},
			{Etichetta: "Scarti", Href: "/admin/scarti", Titolo: "Scarti"},
		}})
	}
	return nav
}

// Nav è il metodo che il template chiama. Passa dall'utente della vista, non da un campo
// riempito a mano: un campo riempito a mano è un campo che qualche handler dimentica.
func (v vista) Nav() []sezioneNav { return navPer(v.Utente) }
