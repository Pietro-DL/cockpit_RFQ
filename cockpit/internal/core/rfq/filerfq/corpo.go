package filerfq

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

// MaxCorpo: il corpo JSON dei gesti della Sorgente (EM §10.9: ≤ 64 KiB, 413 TROPPO_GRANDE).
const MaxCorpo = 64 << 10

// MaxMotivo: il motivo di un gesto dei file, dopo l'NFC (ck_fre_motivo: 1..1000).
const MaxMotivo = 1000

// CorpoGesto e' il corpo di S2, S3 e S4 (EM §10.9). Campi che il gesto non prevede sono un 422 (DisallowUnknownFields):
// in particolare nessun campo percorso (percorso, path_staging, radice…: D23:234).
type CorpoGesto struct {
	VersioneAttesa     *int32     `json:"versione_attesa"`
	ChiaveIdempotenza  *uuid.UUID `json:"chiave_idempotenza"`
	Motivo             *string    `json:"motivo,omitempty"`
	ImprontaDipendenze *string    `json:"impronta_dipendenze,omitempty"`
	NVoci              *int       `json:"n_voci,omitempty"`
	ImprontaVoci       *string    `json:"impronta_voci,omitempty"`
	Scollega           *bool      `json:"scollega,omitempty"`
	ConfermaUnicaCopia *bool      `json:"conferma_unica_copia,omitempty"`
	// Perimetro: per «Metti in Altri file» e «Riattiva» di un archivio (U2 §3.10): contenitore (solo lo ZIP, le voci
	// restano dove sono) | contenitore_e_voci (lo ZIP e le voci, con n_voci e impronta_voci). Assente: la forma di
	// prima, con n_voci e impronta_voci per un archivio (lo ZIP e le voci).
	Perimetro *string `json:"perimetro,omitempty"`
}

// I perimetri di un gesto su un archivio (U2 §3.10).
const (
	PerimetroContenitore      = "contenitore"
	PerimetroContenitoreEVoci = "contenitore_e_voci"
)

var reImpronta = regexp.MustCompile(`^[0-9a-f]{64}$`)

// LeggiCorpo legge e controlla il corpo di un gesto della Sorgente, senza leggere il database (EM §10.9): JSON stretto,
// un valore solo, al piu' MaxCorpo byte (413), i campi del gesto e le loro forme (422 CORPO_NON_VALIDO con il campo;
// 422 MOTIVO_OBBLIGATORIO per «Scarta» senza motivo). Il motivo esce in NFC (un testo umano, EM §3.10).
func LeggiCorpo(r io.Reader, gesto string) (CorpoGesto, *Rifiuto) {
	var c CorpoGesto
	grezzo, err := io.ReadAll(io.LimitReader(r, MaxCorpo+1))
	if err != nil {
		return c, corpoNonValido("", "Niente è cambiato: il corpo della richiesta non si legge.")
	}
	if len(grezzo) > MaxCorpo {
		return c, rifiuto(http.StatusRequestEntityTooLarge, "TROPPO_GRANDE",
			fmt.Sprintf("Niente è cambiato: il corpo supera %d KiB.", MaxCorpo>>10), nil)
	}
	if !utf8.Valid(grezzo) {
		return c, corpoNonValido("", "Niente è cambiato: il corpo non è UTF-8.")
	}
	dec := json.NewDecoder(bytes.NewReader(grezzo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, corpoNonValido(campoDiErrore(err), "Niente è cambiato: il corpo non è nella forma del gesto ("+erroreLeggibile(err)+").")
	}
	if dec.More() {
		return c, corpoNonValido("", "Niente è cambiato: il corpo contiene più di un valore JSON.")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return c, corpoNonValido("", "Niente è cambiato: dopo il corpo c'è altro testo.")
	}
	if c.VersioneAttesa == nil || *c.VersioneAttesa < 0 {
		return c, corpoNonValido("versione_attesa", "Niente è cambiato: manca la versione attesa del file (0 se non ha uno stato).")
	}
	if c.ChiaveIdempotenza == nil || c.ChiaveIdempotenza.Version() != 4 || c.ChiaveIdempotenza.Variant() != uuid.RFC4122 {
		return c, corpoNonValido("chiave_idempotenza", "Niente è cambiato: la chiave d'intento deve essere un UUID v4.")
	}
	if c.Motivo != nil {
		m := strings.TrimSpace(norm.NFC.String(*c.Motivo))
		if m == "" {
			c.Motivo = nil
		} else {
			if utf8.RuneCountInString(m) > MaxMotivo {
				return c, corpoNonValido("motivo", fmt.Sprintf("Niente è cambiato: il motivo supera %d caratteri.", MaxMotivo))
			}
			c.Motivo = &m
		}
	}
	if gesto == GestoScarta && c.Motivo == nil {
		return c, rifiuto(http.StatusUnprocessableEntity, "MOTIVO_OBBLIGATORIO", "«Scarta» vuole il motivo: niente è cambiato.",
			extra("campo", "motivo"))
	}
	if gesto == GestoScarta || gesto == GestoAltriFile {
		if c.ImprontaDipendenze == nil || !reImpronta.MatchString(*c.ImprontaDipendenze) {
			return c, corpoNonValido("impronta_dipendenze", "Niente è cambiato: manca l'impronta delle dipendenze che hai visto (dal dettaglio della Sorgente).")
		}
	} else if c.ImprontaDipendenze != nil {
		return c, corpoNonValido("impronta_dipendenze", "Niente è cambiato: «Riattiva» non porta l'impronta delle dipendenze.")
	}
	if (c.NVoci == nil) != (c.ImprontaVoci == nil) {
		return c, corpoNonValido("n_voci", "Niente è cambiato: il perimetro di un archivio vuole insieme n_voci e impronta_voci.")
	}
	if c.NVoci != nil && (*c.NVoci < 0 || !reImpronta.MatchString(*c.ImprontaVoci)) {
		return c, corpoNonValido("impronta_voci", "Niente è cambiato: il perimetro dell'archivio non è nella sua forma.")
	}
	if gesto != GestoScarta && c.ConfermaUnicaCopia != nil {
		return c, corpoNonValido("conferma_unica_copia", "Niente è cambiato: la conferma dell'unica copia è solo di «Scarta».")
	}
	if gesto == GestoRiattiva && c.Scollega != nil {
		return c, corpoNonValido("scollega", "Niente è cambiato: «Riattiva» non scollega niente.")
	}
	if c.Perimetro != nil {
		switch {
		case gesto == GestoScarta:
			return c, corpoNonValido("perimetro", "Niente è cambiato: «Scarta» non ha un perimetro (uno ZIP si scarta solo senza file estratti attivi).")
		case *c.Perimetro == PerimetroContenitore && (c.NVoci != nil || c.ImprontaVoci != nil):
			return c, corpoNonValido("n_voci", "Niente è cambiato: con il solo ZIP il perimetro delle voci non c'è.")
		case *c.Perimetro == PerimetroContenitoreEVoci && c.NVoci == nil:
			return c, corpoNonValido("n_voci", "Niente è cambiato: lo ZIP con i contenuti vuole n_voci e impronta_voci.")
		case *c.Perimetro != PerimetroContenitore && *c.Perimetro != PerimetroContenitoreEVoci:
			return c, corpoNonValido("perimetro", "Niente è cambiato: il perimetro è contenitore oppure contenitore_e_voci.")
		}
	}
	return c, nil
}

// improntaCorpo e' l'impronta dell'intento di un gesto della Sorgente (EM §8.4; giro 1, SIC-07): sha256 del JSON canonico
// di {gesto, thread_id, allegato_id del percorso, corpo}. La chiave d'intento non entra nel corpo: e' la chiave stessa.
func improntaCorpo(gesto string, thread, allegato uuid.UUID, c CorpoGesto) (string, error) {
	corpo := c
	corpo.ChiaveIdempotenza = nil
	return improntaDi(struct {
		Gesto      string     `json:"gesto"`
		ThreadID   uuid.UUID  `json:"thread_id"`
		AllegatoID uuid.UUID  `json:"allegato_id"`
		Corpo      CorpoGesto `json:"corpo"`
	}{gesto, thread, allegato, corpo})
}

func (c CorpoGesto) scollega() bool   { return c.Scollega != nil && *c.Scollega }
func (c CorpoGesto) unicaCopia() bool { return c.ConfermaUnicaCopia != nil && *c.ConfermaUnicaCopia }
func (c CorpoGesto) motivo() string {
	if c.Motivo == nil {
		return ""
	}
	return *c.Motivo
}

// soloContenitore: il gesto riguarda solo l'archivio, non le sue voci (perimetro contenitore, U2 §3.10).
func (c CorpoGesto) soloContenitore() bool {
	return c.Perimetro != nil && *c.Perimetro == PerimetroContenitore
}

// campoDiErrore: il campo di un errore di json (tipo sbagliato o campo sconosciuto), se si sa.
func campoDiErrore(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return te.Field
	}
	msg := err.Error()
	if i := strings.Index(msg, `unknown field "`); i >= 0 {
		rest := msg[i+len(`unknown field "`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			return rest[:j]
		}
	}
	return ""
}

func erroreLeggibile(err error) string {
	if c := campoDiErrore(err); c != "" {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return "il campo «" + c + "» non ha il tipo giusto"
		}
		return "campo sconosciuto «" + c + "»"
	}
	return "JSON non valido"
}

// SegnoPercorso: cio' che prende il posto di un percorso in un testo che va al browser.
const SegnoPercorso = "‹percorso›"

// SenzaPercorsi toglie i percorsi da un testo che va al browser (un errore del sistema operativo, il messaggio di un
// worker, l'esito di un evento): nessun percorso esce verso il browser (EM §10.9; T-15). Un percorso comincia con una
// lettera di unita' a inizio parola («C:\», «C:/»), con «\\» (UNC, anche «\\?\…»), oppure con «/» dopo uno spazio, una
// virgoletta o una parentesi seguito da un'altra «/» (un percorso assoluto POSIX). Finisce a «: » (due punti seguiti
// da uno spazio o dalla fine: il separatore dei messaggi d'errore, «open C:\…\x.pdf: Accesso negato»; un percorso di
// Windows non ha altri due punti, e quelli di una seconda lettera di unita' non lo fermano), a una virgoletta, a «<»,
// «>», «|», a capo o alla fine del testo: i percorsi con gli spazi («C:\Users\Nome Cognome\…», «\\nas\PREVENTIVI DA
// FARE\…») escono interi (rilievo della revisione: la versione di prima si fermava al primo spazio). Il prezzo e' che
// il resto della frase dopo un percorso senza «: » si perde con lui: meglio una frase corta che un pezzo di percorso.
func SenzaPercorsi(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		n := inizioPercorso(s, i)
		if n == 0 {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i + n
		for j < len(s) && !fineDelPercorso(s, j) {
			j++
		}
		b.WriteString(SegnoPercorso)
		i = j
	}
	return b.String()
}

// fineDelPercorso: alla posizione j di s finisce un percorso (vedi SenzaPercorsi).
func fineDelPercorso(s string, j int) bool {
	switch s[j] {
	case '"', '\'', '<', '>', '|', '\r', '\n':
		return true
	case ':':
		return j+1 == len(s) || s[j+1] == ' ' || s[j+1] == '\t'
	}
	return false
}

// inizioPercorso: la lunghezza dell'inizio di un percorso alla posizione i di s («\\?\C:\», «C:\», «\\», «/x/»), o 0.
func inizioPercorso(s string, i int) int {
	lettera := func(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
	cifra := func(c byte) bool { return c >= '0' && c <= '9' }
	barra := func(c byte) bool { return c == '\\' || c == '/' }
	r := s[i:]
	switch {
	case strings.HasPrefix(r, `\\?\`) || strings.HasPrefix(r, `\\.\`):
		// \\?\C:\…, \\?\UNC\…: il prefisso intero, con la lettera e i suoi due punti
		if len(r) >= 7 && lettera(r[4]) && r[5] == ':' && barra(r[6]) {
			return 7
		}
		return 4
	case strings.HasPrefix(r, `\\`):
		return 2
	case len(r) >= 3 && lettera(r[0]) && r[1] == ':' && barra(r[2]) && (i == 0 || !(lettera(s[i-1]) || cifra(s[i-1]) || s[i-1] == '_')):
		return 3
	case r[0] == '/' && (i == 0 || strings.ContainsRune(" \t\"'([=", rune(s[i-1]))):
		// /mnt/staging/…: almeno un'altra «/» dopo un nome, prima di uno spazio o della fine
		for k := 1; k < len(r); k++ {
			switch {
			case r[k] == '/':
				if k > 1 {
					return 1
				}
				return 0
			case r[k] == ' ' || r[k] == '\t' || fineDelPercorso(r, k):
				return 0
			}
		}
	}
	return 0
}
