package confermabom

// sorgente.go: la sorgente STEP valida per R5 (direttiva 39 §2, «Definizione di sorgente valida per R5» [U + T-39]).
// Una funzione sola, che R4 (blocco firmato STEP_SORGENTE_NON_CONFERMATA) e R5 (gate, ricontrollato sotto lucchetto)
// useranno sullo stesso stato; S8 la usa per dire «gia' fatto» e la revoca per sapere che cosa revoca.
//
// La regola [T-39], sulla storia della fonte del prodotto (fonte_step_evento, ruolo prodotto) per lo sha256 dato, in
// ordine di evento:
//   - conferma_documento scritta da S8 (con la chiave d'idempotenza del passo 2) → la sorgente e' confermata, con il suo
//     documento. Una conferma_documento senza chiave (le pagine legacy, «Conferma la fonte» di prima) non basta: e' il
//     legame storico di D-AR04 (iii), che l'ingegnere riconferma da S8; nessun evento S8 si fabbrica dopo;
//   - revoca della sorgente (azione revoca_sorgente, la revoca di S8), revoca dell'autorizzazione legacy (azione
//     revoca, letta come revoca per prudenza) o rifiuto dello stesso contenuto → non piu' valida, finche' S8 non la
//     conferma di nuovo;
//   - una conferma di S8 piu' recente di un altro STEP dello stesso prodotto la supera (superata), finche' S8 non la
//     conferma di nuovo (proposta della corsia contratto, I.2, adottata [T-39]);
//   - il documento confermato deve esserci, essere un cad_3d .stp/.step del prodotto con quello sha256 (EM App. I.2 punto 5;
//     il trigger della 0020 su step_strutturale_id vuole la stessa estensione) e non essere sostituito (superata).
// I motivi sono quelli del blocco STEP_SORGENTE_NON_CONFERMATA proposti dalla corsia contratto: non_confermata,
// revocata, rifiutata, superata, non_coerente (senza_sorgente lo dice chi chiama, per una versione senza sorgente).
// Il controllo e' per contenuto (sha256), come FONTE_NON_CONFERMATA di R2: un'altra occorrenza dello stesso file vale.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// I motivi di una sorgente non valida per R5.
const (
	SorgenteNessunaConfermaS8  = "non_confermata"
	SorgenteRevocata           = "revocata"
	SorgenteRifiutata          = "rifiutata"
	SorgenteDocumentoAssente   = "non_coerente"
	SorgenteDocumentoSostituto = "superata"
	SorgenteSuperata           = "superata"
)

// AzioneRevocaSorgente: l'azione della revoca di S8 in fonte_step_evento (dalla variante 20261019c della 0023; vigente
// la 20261019d).
const AzioneRevocaSorgente = "revoca_sorgente"

// CausaSorgenteErrata: la causa «sorgente errata» (G39-03 [U], 20/10: «slegare lo step e riassociarlo al prodotto giusto»;
// la forma [T-39] corsia persistenza). Sta in testa al motivo, nei campi che ci sono gia' (nessuna colonna nuova, la 0023
// non cambia): il motivo della revoca della sorgente, il motivo della riga conferma_documento di S8 con correzione_sorgente
// e quello della revoca scritta dal gesto «sposta la sorgente su questo prodotto». Dopo R5 (la BOM confermata con la
// sorgente sbagliata) la sorgente nuova fa della conferma successiva una revisione interna («correzione della sorgente»),
// distinta da una revisione del cliente (StatoSorgente.Correzione).
const CausaSorgenteErrata = "sorgente_errata"

// MotivoConCausa: il motivo registrato con la causa in testa («sorgente_errata» oppure «sorgente_errata: testo»). Un
// motivo che ha gia' la causa resta com'e'.
func MotivoConCausa(causa, testo string) string {
	testo = strings.TrimSpace(testo)
	if CausaDelMotivo(testo) == causa {
		return testo
	}
	if testo == "" {
		return causa
	}
	return causa + ": " + testo
}

// CausaDelMotivo: la causa in testa a un motivo registrato ("" se non ce n'e' una nota).
func CausaDelMotivo(m string) string {
	m = strings.TrimSpace(m)
	if m == CausaSorgenteErrata || strings.HasPrefix(m, CausaSorgenteErrata+":") {
		return CausaSorgenteErrata
	}
	return ""
}

// StatoSorgente: la sorgente di uno sha256 per un prodotto. Valida vuol dire che soddisfa il gate di R5 (la struttura
// non e' per questo verificata: direttiva 39 §2, ultimo capoverso).
type StatoSorgente struct {
	Valida bool
	Motivo string // vuoto se valida
	// l'ultima conferma S8 (anche se poi revocata), se c'e'
	Documento    uuid.UUID
	Evento       int64
	ConfermataIl time.Time
	ConfermataDa uuid.UUID
	Gruppo       string
	// Storica: c'e' una conferma_documento senza chiave (pagine legacy) per quel contenuto, senza una S8 dopo
	Storica bool
	// MotivoConferma: il motivo della riga di S8 dell'ultima conferma.
	MotivoConferma string
	// Correzione: l'ultima conferma S8 corregge una sorgente sbagliata con cui la BOM del prodotto era stata confermata
	// (G39-03 [U]: «se la bom viene confermata con lo step sbagliato [...] un eventuale cambio di sorgente strutturale verra
	// trattato da noi come una revisione», interna e non del cliente; forma [T-39]). Vale in due casi soltanto:
	//   - CorrezioneDichiarata: il motivo della riga di S8 ha la causa sorgente_errata, cioe' il corpo di S8 diceva
	//     correzione_sorgente (il gesto «sposta la sorgente», sposta_da, non la scrive: sul prodotto che riceve lo STEP
	//     non e' una correzione della sua BOM);
	//   - nella revisione in corso, cioe' fra la base (l'ultima conferma della BOM del prodotto) e questa S8, c'e' almeno una
	//     revoca di un altro STEP con la causa sorgente_errata, e quello STEP era entrato in una conferma della BOM del
	//     prodotto (UsataDaConferma). Conta qualunque revoca della revisione, non solo l'ultima: dopo la revoca dello STEP
	//     sbagliato della BOM si possono revocare o spostare altre sorgenti sbagliate prima di quella giusta, e la revisione
	//     resta interna (rilievo bloccante della revisione indipendente 3 della corsia interfaccia, giro G39). Una sorgente
	//     sbagliata revocata o spostata prima di R5 non basta: la BOM non e' mai stata confermata con lei, e la revisione che
	//     segue resta quella del cliente (rilievo bloccante della revisione indipendente del giro G39).
	// Una S8 gia' entrata nella base (la conferma piu' recente e' venuta dopo di lei) non corregge piu' nulla: la correzione
	// l'ha gia' fatta quella R5, e Correzione e' falso anche se il motivo la dichiarava (CorrezioneDichiarata resta).
	Correzione bool
	// CorrezioneDichiarata: il motivo della riga di S8 dell'ultima conferma ha la causa sorgente_errata.
	CorrezioneDichiarata bool
}

// SorgenteValida dice se lo sha256 e' una sorgente valida per R5 del prodotto (commento di testa). Sola lettura: chi
// chiama decide la transazione (R5 la chiama sotto il lucchetto della RFQ).
func SorgenteValida(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, sha string) (StatoSorgente, error) {
	sha = strings.TrimSpace(sha)
	out := StatoSorgente{Motivo: SorgenteNessunaConfermaS8}
	ee, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return out, err
	}
	confermata := false
	// errate: le revoche con la causa sorgente_errata di altri STEP del prodotto, in ordine di evento (quelle della
	// revisione in corso le sceglie correzioneDellaRevisione, con la base)
	var errate []revocaConCausa
	for _, e := range ee {
		if e.Ruolo != "prodotto" {
			continue
		}
		if strings.TrimSpace(e.Sha256) != sha {
			if e.Azione == "conferma_documento" && e.ChiaveIdempotenza.Valid {
				if confermata {
					confermata, out.Motivo = false, SorgenteSuperata
				}
			}
			if e.Azione == AzioneRevocaSorgente && CausaDelMotivo(e.Motivo.String) == CausaSorgenteErrata {
				errate = append(errate, revocaConCausa{evento: e.EventoID, sha: strings.TrimSpace(e.Sha256), documento: e.DocumentoID.UUID})
			}
			continue
		}
		switch e.Azione {
		case "conferma_documento":
			if e.ChiaveIdempotenza.Valid && e.DocumentoID.Valid {
				confermata = true
				out.Storica = false
				out.Motivo = ""
				out.MotivoConferma = e.Motivo.String
				out.CorrezioneDichiarata = CausaDelMotivo(e.Motivo.String) == CausaSorgenteErrata
				out.Documento, out.Evento, out.ConfermataIl, out.ConfermataDa, out.Gruppo = e.DocumentoID.UUID, e.EventoID, e.Quando, e.AutoreID, e.Gruppo.String
			} else if !confermata {
				out.Storica = true
			}
		case "revoca", AzioneRevocaSorgente:
			if confermata {
				confermata, out.Motivo = false, SorgenteRevocata
			}
		case "rifiuto":
			if confermata {
				confermata, out.Motivo = false, SorgenteRifiutata
			}
		}
	}
	if !confermata {
		return out, nil
	}
	// la correzione: dichiarata in S8, o una revoca con la causa, nella revisione in corso, di uno STEP entrato in una
	// conferma della BOM (commento del campo)
	if out.Correzione, err = correzioneDellaRevisione(ctx, q, thread, prodotto, ee, out, errate); err != nil {
		return out, err
	}
	d, err := q.GetDocumento(ctx, out.Documento)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (d.ThreadID != thread || !d.ComponenteID.Valid || d.ComponenteID.UUID != prodotto ||
		d.Tipo != db.TipoDocumentoCad3d || !estensioneStep(d.Estensione) || strings.TrimSpace(d.Sha256) != sha)) {
		out.Motivo = SorgenteDocumentoAssente
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if d.SostituitoDa.Valid {
		out.Motivo = SorgenteDocumentoSostituto
		return out, nil
	}
	out.Valida = true
	return out, nil
}

// revocaConCausa: una revoca della sorgente con la causa sorgente_errata (evento, contenuto e documento revocati).
type revocaConCausa struct {
	evento    int64
	sha       string
	documento uuid.UUID
}

// correzioneDellaRevisione: StatoSorgente.Correzione per l'ultima conferma S8 (out.Evento), date la storia della fonte del
// prodotto (ee, in ordine di evento) e le revoche con la causa sorgente_errata di altri STEP (errate).
//
// La base della revisione in corso e' l'ultima conferma della BOM del prodotto (numero piu' alto). Nella storia della
// fonte la base e' l'ultima riga di S8 della sua sorgente (fonte_sha256) non successiva alla conferma (quando <=
// confermato_il); se la base non ha una sorgente S8 (pagine di prima, legacy), contano le righe successive alla conferma
// (quando > confermato_il). Senza conferme della BOM conta tutta la storia (UsataDaConferma dice poi di no, salvo lo STEP
// strutturale scritto dalle pagine di prima).
//   - S8 di adesso dentro la base: la correzione, se c'era, l'ha gia' fatta quella R5 → falso;
//   - S8 con la causa nel motivo (CorrezioneDichiarata) → vero;
//   - altrimenti vero se una revoca con la causa, fra la base e la S8 di adesso, e' di uno STEP entrato in una conferma
//     della BOM del prodotto.
func correzioneDellaRevisione(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, ee []db.ListFonteStepProdottoRow, out StatoSorgente, errate []revocaConCausa) (bool, error) {
	cc, err := q.ListConfermeStrutturali(ctx, db.ListConfermeStrutturaliParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return false, err
	}
	dopoLaBase := func(int64, time.Time) bool { return true }
	if len(cc) > 0 {
		base := cc[0]
		for _, c := range cc[1:] {
			if c.Numero > base.Numero {
				base = c
			}
		}
		var eventoBase int64
		if base.FonteSha256.Valid {
			fs := strings.TrimSpace(base.FonteSha256.String)
			for _, e := range ee {
				if e.Ruolo == "prodotto" && e.Azione == "conferma_documento" && e.ChiaveIdempotenza.Valid && strings.TrimSpace(e.Sha256) == fs &&
					!e.Quando.After(base.ConfermatoIl) && e.EventoID > eventoBase {
					eventoBase = e.EventoID
				}
			}
		}
		if eventoBase > 0 {
			dopoLaBase = func(evento int64, _ time.Time) bool { return evento > eventoBase }
		} else {
			dopoLaBase = func(_ int64, quando time.Time) bool { return quando.After(base.ConfermatoIl) }
		}
	}
	if !dopoLaBase(out.Evento, out.ConfermataIl) {
		return false, nil
	}
	if out.CorrezioneDichiarata {
		return true, nil
	}
	quando := map[int64]time.Time{}
	for _, e := range ee {
		quando[e.EventoID] = e.Quando
	}
	for _, r := range errate {
		if r.evento >= out.Evento || !dopoLaBase(r.evento, quando[r.evento]) {
			continue
		}
		usata, err := UsataDaConferma(ctx, q, thread, prodotto, r.sha, r.documento)
		if err != nil || usata {
			return usata, err
		}
	}
	return false, nil
}

// estensioneStep: l'estensione di uno STEP (.stp o .step, senza distinguere maiuscole), come il trigger della 0020 su
// componente.step_strutturale_id.
func estensioneStep(e string) bool {
	e = strings.ToLower(strings.TrimSpace(e))
	return e == "stp" || e == "step"
}

// UsataDaConferma: lo STEP (contenuto e documento) e' gia' entrato in una conferma della BOM del prodotto: e' il suo STEP
// strutturale (R5, o le pagine di prima), e' la fonte di una sua conferma strutturale, o sta in una baseline congelata.
// La usano la revoca (prima di R5 annulla la bozza), lo spostamento (rifiuto bom_confermata) e la correzione della
// sorgente (Correzione).
func UsataDaConferma(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, sha string, documento uuid.UUID) (bool, error) {
	c, err := q.GetComponente(ctx, prodotto)
	if err != nil {
		return false, err
	}
	if documento != uuid.Nil && c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == documento {
		return true, nil
	}
	cc, err := q.ListConfermeStrutturali(ctx, db.ListConfermeStrutturaliParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return false, err
	}
	for _, x := range cc {
		if x.FonteSha256.Valid && strings.TrimSpace(x.FonteSha256.String) == strings.TrimSpace(sha) {
			return true, nil
		}
	}
	if documento != uuid.Nil {
		vv, err := q.VersioniConLoStep(ctx, uuid.NullUUID{UUID: documento, Valid: true})
		if err != nil {
			return false, err
		}
		if len(vv) > 0 {
			return true, nil
		}
	}
	return false, nil
}
