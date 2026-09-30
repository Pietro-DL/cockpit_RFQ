package censimento

import (
	"fmt"
	"strings"
	"time"

	"promatec/cockpit/internal/platform/db"
)

// Markdown e' l'export del censimento: lo stesso contenuto della pagina, in un file da salvare in `docs/` (i
// dati reali dei clienti stanno solo li', fuori da git). Si scarica dalla pagina: il server non scrive file.
// L'ordine di ogni elenco e' fisso (Aggrega) e l'ora e' una riga sola in testa, cosi' che due export si
// confrontino con un diff: le righe cambiate sono il «prima e dopo» del codice.
func Markdown(c Censimento, letto time.Time) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("# Forme viste\n\n")
	w("- Censimento delle forme (Smistamento, giro 4, fase 4.17a), letto il %s.\n", letto.Format("2006-01-02 15:04"))
	w("- Contiene dati reali dei clienti: si salva in `docs/`, fuori da git.\n")
	w("- Letto in una transazione di sola lettura: %s.\n", siNo(c.SolaLettura))
	w("- Le forme: cifra → 9, lettera → A, separatori tenuti. Ogni stringa si conta una volta per RFQ (per la posta e i file non ancora in una RFQ, una volta per cliente).\n")
	w("- Le teste, le code e le parole dei commerciali sono suggerimenti per scrivere le regole con una persona: nessuna diventa una regola o un tipo da sola.\n")

	w("\n## Domini dei mittenti non censiti\n\n")
	if len(c.DominiNonCensiti) == 0 {
		w("Nessuno.\n")
	} else {
		w("| Dominio | Mail |\n|---|---|\n")
		for _, v := range c.DominiNonCensiti {
			w("| %s | %d |\n", cella(v.Testo), v.N)
		}
	}

	for _, s := range c.Schede {
		scheda(&b, s)
	}
	return b.String()
}

func scheda(b *strings.Builder, s Scheda) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	w("\n## %s — %s", testo(s.Cliente.Nome), testo(s.Cliente.Ragione))
	if !s.Cliente.Attivo {
		w(" (non attivo)")
	}
	w("\n\n")
	w("- Righe lette: mail %d, file tecnici %d, nodi STEP %d, campi del cartiglio %d (e %d letti con l'OCR, non contati), pezzi decisi %d.\n",
		s.NMessaggi, s.NFile, s.NNodi, s.NCampi, s.NCampiOCR, s.NPezzi)

	w("\n### Forme per fonte\n\n")
	if len(s.Forme) == 0 {
		w("Nessuna.\n")
	} else {
		w("| Forma |")
		for _, f := range Fonti {
			w(" %s |", f)
		}
		w(" Totale | Esempi |\n|---|")
		for range Fonti {
			w("---|")
		}
		w("---|---|\n")
		for _, f := range s.Forme {
			w("| %s |", codice(f.Forma))
			for _, fo := range Fonti {
				w(" %d |", f.Per[fo])
			}
			w(" %d | %s |\n", f.Totale, esempiMd(f.Esempi))
		}
		resto(b, s.AltreForme, "forme", "stringhe")
	}

	aggiunteMd(b, "Code dopo un codice deciso", "Coda", s.Code, s.AltreCode)
	aggiunteMd(b, "Teste prima di un codice deciso", "Testa", s.Teste, s.AltreTeste)
	w("\n- Stringhe uguali a un codice deciso della stessa RFQ: %d.\n", s.Uguali)

	w("\n### Il motore di oggi sulla posta\n\n")
	w("- Mail: %d; con almeno un codice proponibile: %d; codici proponibili: %d; altri numeri mostrati senza proporli: %d.\n",
		s.Mail.Messaggi, s.Mail.ConCodice, s.Mail.Proponibili, s.Mail.Altri)
	w("- Riferimento del cliente trovato dalla sua regola in %d mail.\n", s.Mail.ConRiferimento)
	w("- Accanto all'arrivo, sulle %d mail lette dall'ingest con un'estrazione salvata: codici salvati allora %d; trovati oggi e non allora %d%s; salvati allora e non trovati oggi %d%s.\n",
		s.Mail.Interpretati, s.Mail.Salvati, s.Mail.Nuovi, traParentesi(s.Mail.EsempiNuovi), s.Mail.Persi, traParentesi(s.Mail.EsempiPersi))
	w("- Lette all'arrivo con «ignora» e senza codici salvati, fuori dal confronto (il triage puo' non averle estratte: controparte ambigua, posta non di lavoro): %d.\n",
		s.Mail.SenzaEstrazione)
	vociMd(b, "", "Forma del riferimento", "Mail", s.Mail.FormeRiferimento, Resto{})

	w("\n### Il motore di oggi sui nomi dei file\n\n")
	w("- Nomi di file tecnici: %d (uno per RFQ: la lettura dipende solo dal nome). Con un documento deciso che porta un codice: %d file (ogni contenuto con la sua decisione); codice letto uguale: %d; revisione letta uguale: %d.\n",
		s.Nome.File, s.Nome.Confrontati, s.Nome.CodiceUguale, s.Nome.RevUguale)
	vociMd(b, "", "Lettura del nome", "File", s.Nome.Letture, s.Nome.AltreLetture)
	if len(s.Nome.Diversi) > 0 {
		w("\nLetture diverse dalla decisione:\n\n| File | Letto | Deciso |\n|---|---|---|\n")
		for _, d := range s.Nome.Diversi {
			w("| %s | %s | %s |\n", codice(d.Nome), codice(d.Letto), codice(d.Deciso))
		}
		if s.Nome.AltriDiversi > 0 {
			w("\n- Altre %d letture diverse.\n", s.Nome.AltriDiversi)
		}
	}
	w("\n- File con una proposta di file salvata: %d; codice e revisione della proposta uguali alla lettura del nome di oggi: %d (la proposta puo' venire anche dallo STEP o dal cartiglio).\n",
		s.Nome.ConProposta, s.Nome.PropostaUguale)
	if len(s.Nome.ProposteDiverse) > 0 {
		w("\n| File | Letto oggi dal nome | Proposta salvata |\n|---|---|---|\n")
		for _, d := range s.Nome.ProposteDiverse {
			w("| %s | %s | %s |\n", codice(d.Nome), codice(d.Letto), codice(d.Deciso))
		}
		if s.Nome.AltreProposteDiverse > 0 {
			w("\n- Altre %d proposte diverse.\n", s.Nome.AltreProposteDiverse)
		}
	}

	w("\n### Commerciali decisi\n\n")
	w("- Pezzi decisi: %d particolari commerciali, %d altri.\n", s.Firme.Commerciali, s.Firme.Altri)
	if len(s.Firme.Forme) > 0 {
		w("\n| Forma del codice (con le lettere) | Commerciali | Altri | Esempi |\n|---|---|---|---|\n")
		for _, f := range s.Firme.Forme {
			w("| %s | %d | %d | %s |\n", codice(f.Testo), f.Commerciali, f.Altri, esempiMd(f.Esempi))
		}
		resto(b, s.Firme.AltreForme, "forme", "pezzi")
	}
	if len(s.Firme.Parole) > 0 {
		w("\n| Parola dei nomi | Commerciali | Altri |\n|---|---|---|\n")
		for _, f := range s.Firme.Parole {
			w("| %s | %d | %d |\n", cella(f.Testo), f.Commerciali, f.Altri)
		}
		resto(b, s.Firme.AltreParole, "parole", "pezzi")
	}

	w("\n### Minuteria: proposto e deciso\n\n")
	m := s.Minuteria
	if !m.Proposte {
		w("- Il motore non propone ancora il tipo (arriva con la fase 4.4a.1): si contano solo le decisioni.\n")
	} else {
		w("- Proposte confermate: %d; scartate dall'ingegnere: %d.\n", m.Confermate, m.Scartate)
	}
	w("- Particolari commerciali decisi senza una proposta: %d; altri pezzi: %d.\n", m.NonProposte, m.Altri)
	righeMinuteria(b, "Proposte scartate", m.Scarti, m.AltriScarti)
	righeMinuteria(b, "Commerciali decisi senza una proposta", m.NonViste, m.AltriNonVisti)

	vociMd(b, "Oggetti delle mail", "Forma dell'oggetto", "Mail", s.Oggetti, s.AltriOggetti)
	vociMd(b, "Numeri accanto alle parole di riferimento", "Parola e forma", "Mail", s.Riferimenti, s.AltriRiferimenti)
	vociMd(b, "Mittenti del dominio non censiti come buyer", "Indirizzo", "Mail", s.Mittenti, s.AltriMittenti)
}

func aggiunteMd(b *strings.Builder, titolo, colonna string, v []Aggiunta, r Resto) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	w("\n### %s\n\n", titolo)
	if len(v) == 0 {
		w("Nessuna.\n")
		return
	}
	w("| %s | Forma | Volte | Pezzi | RFQ | Fonti | Alias candidato | Esempi |\n|---|---|---|---|---|---|---|---|\n", colonna)
	for _, a := range v {
		alias := "no"
		if a.Alias {
			alias = "**sì**"
		}
		w("| %s | %s | %d | %d | %d | %s | %s | %s |\n", codice(a.Testo), codice(a.Forma), a.N, a.Pezzi, a.RFQ,
			strings.Join(a.Fonti, ", "), alias, esempiMd(a.Esempi))
	}
	resto(b, r, "voci", "stringhe")
}

func vociMd(b *strings.Builder, titolo, colonna, quante string, v []Voce, r Resto) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	if titolo != "" {
		w("\n### %s\n\n", titolo)
		if len(v) == 0 {
			w("Nessuno.\n")
			return
		}
	} else if len(v) == 0 {
		return
	} else {
		w("\n")
	}
	w("| %s | %s | Esempi |\n|---|---|---|\n", colonna, quante)
	for _, x := range v {
		w("| %s | %d | %s |\n", codice(x.Testo), x.N, esempiMd(x.Esempi))
	}
	resto(b, r, "voci", "occorrenze")
}

func righeMinuteria(b *strings.Builder, titolo string, v []RigaMinuteria, altri int) {
	if len(v) == 0 {
		return
	}
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	w("\n%s:\n\n| Codice | Nome | Deciso | Proposto | Motivo |\n|---|---|---|---|---|\n", titolo)
	for _, r := range v {
		w("| %s | %s | %s | %s | %s |\n", codice(r.Codice), cella(r.Nome), NomeTipo(r.Deciso), NomeTipo(r.Proposto), cella(r.Motivo))
	}
	if altri > 0 {
		w("\n- Altri %d.\n", altri)
	}
}

func resto(b *strings.Builder, r Resto, voci, n string) {
	if r.Voci > 0 {
		fmt.Fprintf(b, "\n- Altre %d %s (%d %s).\n", r.Voci, voci, r.N, n)
	}
}

// NomeTipo e' il tipo di un pezzo con le parole della Distinta («particolare commerciale» ovunque: scelta 4
// delle domande del giro 4); «—» per nessun tipo.
func NomeTipo(t db.TipoComponente) string {
	switch t {
	case "":
		return "—"
	case db.TipoComponenteFinito:
		return "prodotto"
	case db.TipoComponenteSottoassieme:
		return "assieme"
	case db.TipoComponenteSciolto:
		return "particolare"
	case db.TipoComponenteCommerciale:
		return "particolare commerciale"
	}
	return string(t)
}

func siNo(v bool) string {
	if v {
		return "sì"
	}
	return "no"
}

// cella e' un testo dentro una cella: una barra verticale chiuderebbe la cella, un a capo la riga.
func cella(s string) string {
	s = strings.NewReplacer("|", `\|`, "\r", " ", "\n", " ").Replace(s)
	return strings.TrimSpace(s)
}

// codice e' un testo fra apici inversi: un «_» di un codice non diventa un corsivo.
func codice(s string) string {
	s = cella(strings.ReplaceAll(s, "`", "'"))
	if s == "" {
		return ""
	}
	return "`" + s + "`"
}

// testo e' un nome in un titolo: senza a capo.
func testo(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(s))
}

// traParentesi sono gli esempi fra parentesi, o niente.
func traParentesi(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return " (" + esempiMd(v) + ")"
}

func esempiMd(v []string) string {
	out := make([]string, 0, len(v))
	for _, e := range v {
		out = append(out, codice(e))
	}
	return strings.Join(out, ", ")
}
