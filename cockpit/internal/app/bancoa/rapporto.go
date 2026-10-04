package bancoa

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/dataset"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Rapporto: l'uscita di un'esecuzione del banco, con l'esito in testa (R44). Si scrive in JSON canonico e in
// un riepilogo di testo; contiene dati privati (ID dei casi, profili, UUID, percorsi del dataset), quindi la
// sua cartella sta fuori dal repository e non si incolla in commit, PR o note pubbliche (par.3.7.5).
type Rapporto struct {
	VersioneRapporto int             `json:"versione_rapporto"`
	Modalita         string          `json:"modalita"`
	Esito            Esito           `json:"esito"`
	Motivo           string          `json:"motivo,omitempty"` // perché non eseguito
	Differenze       int             `json:"differenze"`
	Manifest         string          `json:"manifest"`
	Sha256Manifest   string          `json:"sha256_manifest,omitempty"`
	Sha256Attesi     string          `json:"sha256_attesi,omitempty"`
	Sha256Indice     string          `json:"sha256_indice,omitempty"`
	ImprontaIndice   string          `json:"impronta_indice,omitempty"`
	VersioneLimiti   string          `json:"versione_limiti,omitempty"` // la versione_limiti dell'indice (R43 B)
	VersioneAttesi   int             `json:"versione_attesi,omitempty"` // dalla testata: è l'hash a fare da versione
	Versioni         Versioni        `json:"versioni"`
	Controlli        []Controllo     `json:"controlli"`
	Regole           *RapportoRegole `json:"regole,omitempty"`
	Casi             *RapportoCasi   `json:"casi,omitempty"`
}

// Versioni: le versioni del codice con cui il rapporto è stato prodotto, come la riga di log dell'anteprima
// (par.3.6.4). Quella dei limiti non sta qui: la dichiara l'indice.
type Versioni struct {
	Rapporto           int    `json:"rapporto"`
	Manifest           int    `json:"manifest"`
	Canonicalizzazione string `json:"canonicalizzazione"`
	SchemaGrammatiche  int    `json:"schema_grammatiche"`
	Indice             int    `json:"indice"`
	Capacita           string `json:"capacita"`
	Algoritmo          string `json:"algoritmo"`
}

// Controllo: una verifica del banco, eseguita o no. Mai «conforme» se un controllo è non eseguito (R44).
type Controllo struct {
	Nome       string `json:"nome"`
	Stato      string `json:"stato"` // eseguito | non_eseguito
	Differenze int    `json:"differenze"`
	Motivo     string `json:"motivo,omitempty"`
}

// Gli stati di un controllo.
const (
	ControlloEseguito    = "eseguito"
	ControlloNonEseguito = "non_eseguito"
)

// RapportoCasi: l'esito della modalità «casi» (A1a-P2).
type RapportoCasi struct {
	Conteggi        ConteggiCasi           `json:"conteggi"`
	ClientiAttivi   int                    `json:"clienti_attivi"`
	ClientiScartati int                    `json:"clienti_scartati"`
	Scarti          []evidenze.Diagnostica `json:"scarti,omitempty"` // le diagnostiche delle grammatiche scartate
	Casi            []EsitoCaso            `json:"casi"`
}

// PrimaRiga: «ESITO: …», la prima riga del riepilogo (R44).
func (r Rapporto) PrimaRiga() string {
	switch r.Esito {
	case EsitoConforme:
		return "ESITO: ESEGUITO — conforme"
	case EsitoConDifferenze:
		return fmt.Sprintf("ESITO: ESEGUITO — con differenze (%d)", r.Differenze)
	}
	return "ESITO: NON ESEGUITO — " + r.Motivo
}

// Testo: il riepilogo leggibile. La prima riga è l'esito; poi modalità, manifest, sha256 di attesi e indice,
// versione dei limiti, versioni, controlli e il dettaglio della modalità. Per ogni caso non passato: atteso
// e ottenuto per chiave, le letture con famiglia e forma, le forme riservate sul selettore, le diagnostiche.
func (r Rapporto) Testo() string {
	var b strings.Builder
	riga := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	riga("%s", r.PrimaRiga())
	riga("modalità: %s", r.Modalita)
	riga("manifest: %s (sha256 %s)", r.Manifest, vuotoONo(r.Sha256Manifest))
	riga("attesi: sha256 %s, versione_attesi %d", vuotoONo(r.Sha256Attesi), r.VersioneAttesi)
	riga("indice: sha256 %s, impronta %s", vuotoONo(r.Sha256Indice), vuotoONo(r.ImprontaIndice))
	riga("versione_limiti: %s", vuotoONo(r.VersioneLimiti))
	v := r.Versioni
	riga("versioni: rapporto %d, manifest %d, %s, schema %d, indice %d, %s, %s",
		v.Rapporto, v.Manifest, v.Canonicalizzazione, v.SchemaGrammatiche, v.Indice, v.Capacita, v.Algoritmo)
	riga("controlli:")
	for _, c := range r.Controlli {
		s := "  - " + c.Nome + ": " + c.Stato
		if c.Differenze > 0 {
			s += fmt.Sprintf(", %d differenze", c.Differenze)
		}
		if c.Motivo != "" {
			s += " — " + c.Motivo
		}
		riga("%s", s)
	}
	if r.Regole != nil {
		testoRegole(riga, *r.Regole)
	}
	if r.Casi != nil {
		testoCasi(riga, *r.Casi)
	}
	return b.String()
}

func vuotoONo(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func testoRegole(riga func(string, ...any), rr RapportoRegole) {
	riga("regole: %d clienti attivi, %d scartati, %d incoerenze esempio/caso, %d lacune di copertura",
		rr.ClientiAttivi, rr.ClientiScartati, rr.Incoerenze, rr.Lacune)
	for _, d := range rr.IndiceNonValido {
		riga("  indice non valido: %s %s %s", d.Codice, d.Percorso, d.Messaggio)
	}
	for _, c := range rr.Clienti {
		riga("  cliente %s (%s), file %s: %s, hash %s", c.ClienteID, strings.Join(c.Profili, ", "), c.File, c.Stato, vuotoONo(c.Hash))
		for _, f := range c.Famiglie {
			riga("    famiglia %s: ruoli %s; categorie %s; forme attive %s; riservate %s", f.ID,
				elenco(f.Ruoli), elenco(f.Categorie), elenco(f.FormeAttive), elenco(f.FormeRiservate))
		}
		for _, x := range c.Riserve {
			riga("    riserva %s: %s", x.ID, x.Motivo)
		}
		riga("    esempi: %d, verificati %d, non verificati %d %s", c.Esempi.Totali, c.Esempi.Verificati,
			c.Esempi.NonVerificati, elencoSeC(c.Esempi.NonVerificatiID))
		for _, l := range c.Lacune {
			riga("    lacuna: %s/%s su %s, manca %s", l.Famiglia, l.Regola, l.Selettore, l.Manca)
		}
		for _, k := range c.Coerenza {
			if !k.Coerente {
				riga("    incoerente: esempio %s/%s, caso %s: %s", k.Famiglia, k.Esempio, k.Caso, strings.Join(k.Motivi, "; "))
			}
		}
		for _, d := range c.Diagnostiche {
			riga("    %s %s %s %s", d.Gravita, d.Codice, d.Percorso, d.Messaggio)
		}
	}
}

func elencoSeC(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return "(" + strings.Join(s, ", ") + ")"
}

func testoCasi(riga func(string, ...any), rc RapportoCasi) {
	n := rc.Conteggi
	riga("casi: %d — passati %d, parziali %d, falliti %d, riservati %d, rimandati %d",
		n.Totale, n.Passati, n.Parziali, n.Falliti, n.Riservati, n.Rimandati)
	riga("grammatiche: %d clienti attivi, %d scartati", rc.ClientiAttivi, rc.ClientiScartati)
	for _, d := range rc.Scarti {
		riga("  scarto: %s %s %s", d.Codice, d.Percorso, d.Messaggio)
	}
	for _, e := range rc.Casi {
		if e.Esito == CasoPassato {
			continue
		}
		s := fmt.Sprintf("  caso %s (%s, %s): %s", e.ID, e.Profilo, e.Selettore, e.Esito)
		if e.Motivo != "" {
			s += " — " + e.Motivo
		}
		if e.DipendeDa != "" {
			s += " [dipende_da " + e.DipendeDa + "]"
		}
		riga("%s", s)
		for _, k := range e.Chiavi {
			if k.Stato == ChiavePassata {
				continue
			}
			t := fmt.Sprintf("    %s [%s, %s]: atteso %s", k.Chiave, k.Sessione, k.Stato, k.Atteso)
			if k.Ottenuto != "" {
				t += ", ottenuto " + k.Ottenuto
			}
			if k.Motivo != "" {
				t += " — " + k.Motivo
			}
			riga("%s", t)
		}
		for _, l := range e.Letture {
			riga("    lettura %s/%s [%d,%d): base %s, stato %s, funzione %s", l.Famiglia, l.Forma, l.Inizio, l.Fine, l.Base, l.Stato, l.Funzione)
		}
		for _, a := range e.Attributi {
			riga("    attributo %s %s: grezzo %q, normalizzato %q", a.ID, a.Stato, a.Grezzo, a.Normalizzato)
		}
		if len(e.Letture) == 0 {
			riga("    nessuna lettura; forme riservate sul selettore: %s", elenco(e.RiservateSulSelettore))
		}
		for _, d := range e.Diagnostiche {
			riga("    %s %s %s", d.Gravita, d.Codice, d.Messaggio)
		}
	}
}

// ScriviRapporto: JSON canonico più un riepilogo in testo, scritti in modo atomico (.tmp, poi Rename) nella
// cartella dei rapporti, che si crea se manca. Il nome dei file lo dà la modalità: <modalità>.json e
// <modalità>.txt. Rifiuta una cartella dentro il modulo: le uscite non entrano mai nel repository.
func ScriviRapporto(cartella string, r Rapporto) error {
	if r.Modalita != ModalitaRegole && r.Modalita != ModalitaCasi {
		return fmt.Errorf("bancoa: modalità %q: il nome del rapporto non si sa", r.Modalita)
	}
	if err := dataset.FuoriDalModulo(cartella); err != nil {
		return err
	}
	if err := os.MkdirAll(cartella, 0o755); err != nil {
		return fmt.Errorf("bancoa: cartella dei rapporti: %w", err)
	}
	js, err := jsoncanonico.Codifica(r)
	if err != nil {
		return fmt.Errorf("bancoa: forma canonica del rapporto: %w", err)
	}
	if err := scriviAtomico(filepath.Join(cartella, r.Modalita+".json"), js); err != nil {
		return err
	}
	return scriviAtomico(filepath.Join(cartella, r.Modalita+".txt"), []byte(r.Testo()))
}

// scriviAtomico: prima il file .tmp accanto, poi Rename: chi legge vede il file vecchio o quello nuovo, mai
// uno a metà.
func scriviAtomico(percorso string, b []byte) error {
	tmp := percorso + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("bancoa: %w", err)
	}
	if err := os.Rename(tmp, percorso); err != nil {
		return errors.Join(fmt.Errorf("bancoa: %w", err), os.Remove(tmp))
	}
	return nil
}
