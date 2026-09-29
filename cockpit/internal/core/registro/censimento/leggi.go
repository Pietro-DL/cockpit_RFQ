package censimento

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// Raccogli legge il database e fa i conti: e' quello che chiamano la pagina «Forme viste» e il suo export.
func Raccogli(ctx context.Context, pool *pgxpool.Pool) (Censimento, error) {
	in, err := Leggi(ctx, pool)
	if err != nil {
		return Censimento{}, err
	}
	return Aggrega(in), nil
}

// Leggi legge tutto in una transazione READ ONLY e REPEATABLE READ: il censimento legge, e non deve poter
// scrivere nemmeno per sbaglio (la transazione lo dice da se', e SolaLettura riporta che cosa ha risposto il
// database, come la calibrazione); le otto letture vedono lo stesso database, anche se nel frattempo arriva
// posta.
func Leggi(ctx context.Context, pool *pgxpool.Pool) (Ingresso, error) {
	var in Ingresso
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return in, fmt.Errorf("censimento: %w", err)
	}
	defer tx.Rollback(ctx)
	var ro string
	if err := tx.QueryRow(ctx, "SHOW transaction_read_only").Scan(&ro); err != nil {
		return in, fmt.Errorf("censimento: %w", err)
	}
	in.SolaLettura = ro == "on"
	q := db.New(tx)

	clienti, err := q.CensimentoClienti(ctx)
	if err != nil {
		return in, fmt.Errorf("censimento, clienti: %w", err)
	}
	for _, c := range clienti {
		in.Clienti = append(in.Clienti, Cliente{ID: c.ClienteID, Nome: c.CartellaNas, Ragione: c.RagioneSociale, Attivo: c.Attivo, Regole: c.Regole})
	}

	messaggi, err := q.CensimentoMessaggi(ctx)
	if err != nil {
		return in, fmt.Errorf("censimento, messaggi: %w", err)
	}
	for _, m := range messaggi {
		in.Messaggi = append(in.Messaggi, Messaggio{Cliente: m.ClienteID, Thread: rfq(m.ThreadID), Oggetto: m.Oggetto, Corpo: m.Corpo,
			Troncato: m.CorpoTroncato, Allegati: m.Allegati, Mittente: m.Mittente, ViaDominio: m.ViaDominio,
			Interpretato: m.Interpretato, Estratto: m.Estratto, Salvati: m.Salvati})
	}

	file, err := q.CensimentoFile(ctx)
	if err != nil {
		return in, fmt.Errorf("censimento, file: %w", err)
	}
	for _, f := range file {
		in.File = append(in.File, File{Cliente: f.ClienteID, Thread: rfq(f.ThreadID), Nome: f.NomeFile, Contenuto: f.Sha256, Deciso: f.Deciso,
			Codice: f.CodiceDeciso, Rev: f.RevDecisa, Proposto: f.Proposto, CodiceProposto: f.CodiceProposto, RevProposta: f.RevProposta})
	}

	nodi, err := q.CensimentoNodi(ctx)
	if err != nil {
		return in, fmt.Errorf("censimento, nodi: %w", err)
	}
	for _, n := range nodi {
		in.Nodi = append(in.Nodi, Nodo{Cliente: n.ClienteID, Thread: n.ThreadID, ID: n.IDGrezzo, Nome: n.NomeGrezzo})
	}

	cartigli, err := q.CensimentoCartigli(ctx)
	if err != nil {
		return in, fmt.Errorf("censimento, cartigli: %w", err)
	}
	for _, c := range cartigli {
		in.Campi = append(in.Campi, CampiDelCartiglio(c.ClienteID, rfq(c.ThreadID), c.Sha256, c.Fatti)...)
	}

	pezzi, err := q.CensimentoPezzi(ctx)
	if err != nil {
		return in, fmt.Errorf("censimento, pezzi: %w", err)
	}
	for _, p := range pezzi {
		nomi := p.Nomi
		if d := strings.TrimSpace(p.Descrizione); d != "" {
			nomi = append([]string{d}, nomi...)
		}
		// Proposto e Motivo restano vuoti: il tipo proposto arriva con la 4.4a.1 (calcolato in lettura
		// nell'albero proposto), e sara' li' che il censimento lo chiedera'
		in.Pezzi = append(in.Pezzi, Pezzo{Cliente: p.ClienteID, Thread: p.ThreadID, Codice: p.Codice, Rev: p.Rev, Nomi: nomi, Deciso: p.Tipo})
	}

	decisi, err := q.CensimentoCodiciDecisi(ctx)
	if err != nil {
		return in, fmt.Errorf("censimento, codici decisi: %w", err)
	}
	for _, d := range decisi {
		in.Decisi = append(in.Decisi, Deciso{Cliente: d.ClienteID, Thread: d.ThreadID, Codice: d.Codice})
	}

	domini, err := q.CensimentoDominiNonCensiti(ctx)
	if err != nil {
		return in, fmt.Errorf("censimento, domini: %w", err)
	}
	for _, d := range domini {
		in.DominiNonCensiti = append(in.DominiNonCensiti, Voce{Testo: d.Dominio, N: int(d.N)})
	}
	return in, nil
}

// CampiDelCartiglio sono i campi codice e numero di disegno del probabile cartiglio nei fatti di un'analisi di
// un PDF, attribuiti a un cliente e a una RFQ. Il JSON del worker lo legge la classificazione
// (CampiIdentificativiDelPDF, con il controllo della versione), non il censimento: i fatti del worker si
// leggono solo li' (F9). Fatti che non si leggono non danno campi: un worker piu' vecchio o piu' nuovo non deve
// fermare il censimento.
func CampiDelCartiglio(cliente, thread uuid.UUID, contenuto string, fatti json.RawMessage) []Campo {
	var out []Campo
	for _, c := range classificazione.CampiIdentificativiDelPDF(fatti) {
		out = append(out, Campo{Cliente: cliente, Thread: thread, Contenuto: contenuto, Etichetta: c.Etichetta, Valore: c.Valore,
			OCR: c.Indizio()})
	}
	return out
}

// rfq e' la RFQ di una riga, uuid.Nil se non ce n'e' una.
func rfq(t uuid.NullUUID) uuid.UUID {
	if t.Valid {
		return t.UUID
	}
	return uuid.Nil
}
