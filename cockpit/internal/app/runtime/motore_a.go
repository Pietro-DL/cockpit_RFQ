// La Distinta nuova del motore A all'avvio: l'indice delle regole di [motore_a], le grammatiche e il file dei casi che
// elenca, letti una volta sola (giro 5, A1d; piano 7.4.5; direttiva A1d dell'11/10, §2.2). Derivato dalla PR #18 di
// Stefano Valo (ec06ca4), con i tipi spostati in transport/web/motore_a e gli stati distinti (Q-A1d-03 = A [U]).

package runtime

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/jsoncanonico"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// regoleLette: ciò che l'avvio legge dalla cartella dell'indice, prima di ogni controllo di contenuto (piano 7.4.5).
//   - Grammatiche: per nome com'è scritto nell'indice; manca il file che non si legge o che esce dalla cartella, e
//     CompilaInsieme lo dà come regole.file_assente (il cliente è scartato, gli altri restano attivi: par.3.6.5).
//   - Fuori: i nomi dell'indice che escono dalla cartella (R55): non si leggono, e il log lo dice.
//   - Casi: la diagnosi del file dei casi (R29 b = C); CasiRaw i byte, solo se letti e con lo sha256 giusto.
type regoleLette struct {
	Indice       grammatica.IndiceRegole
	Sha256Indice string
	Grammatiche  map[string][]byte
	Fuori        []string
	Casi         motorea_web.Casi
	CasiRaw      []byte
}

// erroreIndiceAssente: l'indice indicato non si legge dal disco. Gli altri errori di leggiRegoleMotoreA sono un indice
// che si legge ma è rifiutato.
var erroreIndiceAssente = errors.New("l'indice non si legge")

// leggiRegoleMotoreA è la parte che tocca il disco (piano 7.4.5), separata per le prove con una cartella temporanea:
// l'indice (LeggiIndice: limiti, voci, sha256 presenti), poi i file che elenca, relativi alla cartella dell'indice e
// mai fuori (filepath.IsLocal per il nome, os.Root per i collegamenti che escono: R55). Le barre rovesciate del nome
// valgono come separatori, come nel banco (bancoa.go:355); a differenza del banco, un nome con «..» non si legge.
// Lo sha256 delle grammatiche lo controlla CompilaInsieme (regole.sha256_discorde); quello del file dei casi lo
// controlla questa funzione, con la stessa impronta (jsoncanonico.Impronta, maiuscole delle cifre indifferenti),
// perché nessuna funzione del core lo fa. Un errore solo per l'indice: assente (erroreIndiceAssente) o rifiutato.
func leggiRegoleMotoreA(indice string) (regoleLette, error) {
	raw, err := os.ReadFile(indice)
	if err != nil {
		return regoleLette{}, fmt.Errorf("%w: %s: %w", erroreIndiceAssente, indice, err)
	}
	ind, err := grammatica.LeggiIndice(raw)
	if err != nil {
		return regoleLette{}, fmt.Errorf("indice %s: %w", indice, err)
	}
	// os.Root (Go 1.24, ReadFile da Go 1.25; il modulo chiede la 1.26): un nome che passa IsLocal ma è un
	// collegamento verso fuori dalla cartella non si apre (T-A1d-16).
	root, err := os.OpenRoot(filepath.Dir(indice))
	if err != nil {
		return regoleLette{}, fmt.Errorf("%w: %s: %w", erroreIndiceAssente, indice, err)
	}
	defer root.Close()
	r := regoleLette{Indice: ind, Sha256Indice: jsoncanonico.Impronta(raw), Grammatiche: map[string][]byte{},
		Casi: motorea_web.Casi{Stato: motorea_web.CasiNonDichiarati}}
	for _, v := range ind.Grammatiche {
		nome, ok := nomeLocale(v.File)
		if !ok {
			r.Fuori = append(r.Fuori, v.File)
			continue
		}
		if b, err := root.ReadFile(nome); err == nil {
			r.Grammatiche[v.File] = b
		}
	}
	if c := ind.Casi; c != nil {
		r.Casi.File = c.File
		if nome, ok := nomeLocale(c.File); !ok {
			r.Casi.Stato, r.Casi.Motivo = motorea_web.CasiFuoriCartella, "il file dei casi esce dalla cartella dell'indice"
		} else if b, err := root.ReadFile(nome); err != nil {
			r.Casi.Stato, r.Casi.Motivo = motorea_web.CasiNonLeggibili, "il file dei casi non si legge: "+err.Error()
		} else if imp := jsoncanonico.Impronta(b); !strings.EqualFold(imp, c.Sha256) {
			r.Casi.Stato, r.Casi.Motivo = motorea_web.CasiSha256Discorde, "il file dei casi ha uno sha256 diverso da quello dell'indice"
		} else {
			r.Casi.Stato, r.Casi.Impronta, r.CasiRaw = motorea_web.CasiLetti, imp, b
		}
	}
	return r, nil
}

// nomeLocale: il nome di un file dell'indice nella cartella dell'indice; falso se ne esce (R55).
func nomeLocale(f string) (string, bool) {
	n := filepath.FromSlash(strings.ReplaceAll(strings.TrimSpace(f), `\`, "/"))
	return n, n != "" && filepath.IsLocal(n)
}

// preparaMotoreA legge una volta, all'avvio, l'indice delle regole del motore A, le grammatiche e il file dei casi che
// l'indice elenca (R29 b = C); compila le grammatiche con i limiti dell'indice (R43 B) e legge gli ingressi con
// valutazione.LeggiIngressi. Le regole NON vengono da cliente.regole, e questa funzione non scrive niente: nessuna
// semina, nessun DB (direttiva §2.3). Non restituisce errori: il Cockpit parte sempre (piano 7.4.5).
//   - Spenta: nessun file aperto, una riga «anteprima motore A spenta», StatoSpenta.
//   - Indice assente o rifiutato: Accesa, lo stato dell'errore, una riga ERROR con il motivo.
//   - Indice valido ma nessuna grammatica attiva: StatoNessunaGrammatica e una riga ERROR (Q-A1d-03 = A [U]).
//   - File dei casi non dichiarato, fuori dalla cartella, illeggibile, con sha256 diverso o non valido: la sua diagnosi
//     in Casi e, se dichiarato e non letto, un avviso; le regole restano.
func preparaMotoreA(cfg config.MotoreA, log *slog.Logger) motorea_web.Regole {
	if !cfg.Anteprima {
		log.Info("anteprima motore A spenta")
		return motorea_web.Regole{Stato: motorea_web.StatoSpenta}
	}
	out := motorea_web.Regole{Accesa: true, Indice: cfg.Regole}
	r, err := leggiRegoleMotoreA(cfg.Regole)
	if err != nil {
		out.Stato = motorea_web.StatoIndiceNonValido
		if errors.Is(err, erroreIndiceAssente) {
			out.Stato = motorea_web.StatoIndiceAssente
		}
		log.Error("anteprima motore A accesa senza regole", "stato", out.Stato, "indice", cfg.Regole, "err", err)
		return out
	}
	out.Sha256Indice, out.VersioneIndice, out.VersioneLimiti = r.Sha256Indice, r.Indice.VersioneIndice, r.Indice.Limiti.Versione
	for _, f := range r.Fuori {
		log.Warn("anteprima motore A: un file dell'indice esce dalla sua cartella e non si legge",
			"indice", cfg.Regole, "file", f)
	}
	ins, diag, err := motorea.CompilaInsieme(r.Indice, r.Grammatiche)
	if err != nil {
		out.Stato = motorea_web.StatoIndiceNonValido
		log.Error("anteprima motore A accesa senza regole: l'indice non attiva nessuna grammatica",
			"stato", out.Stato, "indice", cfg.Regole, "err", err)
		return out
	}
	out.Insieme = &ins
	out.Attivi, out.Scartati = attiviEScartati(ins)
	// Il tetto (60 s) lo controlla già Limiti.Valida, dentro LeggiIndice.
	out.Limite = time.Duration(r.Indice.Limiti.Anteprima.TempoMassimoMs) * time.Millisecond
	out.Casi = r.Casi
	if r.Casi.Stato == motorea_web.CasiLetti {
		if in, err := valutazione.LeggiIngressi(r.CasiRaw); err != nil {
			out.Casi.Stato, out.Casi.Impronta, out.Casi.Motivo = motorea_web.CasiNonValidi, "", err.Error()
		} else {
			out.Ingressi, out.Casi.Numero = in, len(in.Casi)
		}
	}
	if out.Casi.Stato != motorea_web.CasiLetti && out.Casi.Stato != motorea_web.CasiNonDichiarati {
		log.Warn("anteprima motore A: file dei casi non letto, ingressi vuoti (le regole restano)",
			"indice", cfg.Regole, "casi", out.Casi.Stato, "file", out.Casi.File, "motivo", out.Casi.Motivo)
	}
	out.Stato = motorea_web.StatoPronta
	if len(ins.Motori) == 0 {
		out.Stato = motorea_web.StatoNessunaGrammatica
	}
	// La riga dell'avvio (piano 7.4.5; T-A1d-38; direttiva §2.2): stato, le due impronte dell'indice, versioni
	// dell'indice e dei limiti, limite, casi, clienti attivi (UUID) e scartati (UUID e codici), versioni di
	// valutazione e di motorea. Solo UUID e codici: nessun testo dei clienti nel log.
	campi := []any{"stato", out.Stato, "indice", cfg.Regole, "sha256_indice", out.Sha256Indice,
		"impronta_indice", ins.ImprontaIndice, "versione_indice", out.VersioneIndice, "versione_limiti", out.VersioneLimiti,
		"limite", out.Limite, "casi", out.Casi.Stato, "impronta_casi", out.Casi.Impronta, "numero_casi", out.Casi.Numero,
		"clienti_attivi", len(out.Attivi), "attivi", uuidTesto(out.Attivi), "clienti_scartati", len(out.Scartati),
		"scartati", scartiTesto(out.Scartati), "diagnostiche", len(diag),
		"versione_valutazione", valutazione.VersioneValutazione, "versione_motorea", motorea.VersioneAlgoritmo,
		"versione_composizione", motorea.VersioneComposizione,
		"versione_revisione_registrata", motorea.VersioneRevisioneRegistrata, "visibile", "solo al ruolo admin"}
	if out.Stato == motorea_web.StatoNessunaGrammatica {
		log.Error("anteprima motore A accesa senza regole: nessuna grammatica attiva", campi...)
		return out
	}
	log.Info("anteprima motore A accesa", campi...)
	return out
}

// attiviEScartati: i clienti attivi e quelli scartati dell'insieme, in ordine di stringa dell'UUID (le mappe non si
// scorrono in un ordine), con tutti i codici delle diagnostiche di ogni scarto.
func attiviEScartati(ins motorea.InsiemeRegole) ([]uuid.UUID, []motorea_web.Scarto) {
	attivi := make([]uuid.UUID, 0, len(ins.Motori))
	for id := range ins.Motori {
		attivi = append(attivi, id)
	}
	sort.Slice(attivi, func(i, j int) bool { return attivi[i].String() < attivi[j].String() })
	scartati := make([]motorea_web.Scarto, 0, len(ins.Scartati))
	for id, d := range ins.Scartati {
		s := motorea_web.Scarto{Cliente: id}
		for _, x := range d {
			s.Codici = append(s.Codici, x.Codice)
		}
		scartati = append(scartati, s)
	}
	sort.Slice(scartati, func(i, j int) bool { return scartati[i].Cliente.String() < scartati[j].Cliente.String() })
	return attivi, scartati
}

func uuidTesto(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

// scartiTesto: «<uuid> <codice>,<codice>» per ogni scarto, per la riga dell'avvio.
func scartiTesto(ss []motorea_web.Scarto) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Cliente.String() + " " + strings.Join(s.Codici, ",")
	}
	return out
}
