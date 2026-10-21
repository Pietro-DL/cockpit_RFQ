package distintabozza

// La presa in carico del prodotto alla nascita della bozza formato 3 (direttiva 39 §4 [U + T-39]; EM I.8, I.9): oltre
// alla versione 1 costruita dalla partenza, nella stessa transazione,
//  1. le pre-assegnazioni legacy aperte dei nodi diventano PB legacy (conversionep1.go, D-AR14 A [T-39]);
//  2. con la partenza step (il registro della versione) e da-conferma-1 (il registro della conferma formato 3 di
//     partenza, per i file senza AR attiva) le proposte univoche diventano PB motore (pbiniziali3.go, D-PB01 A [U]),
//     solo con la sorgente confermata da S8 (D-AR11 B [U]). vuota, registrato e solo_prodotto non hanno registro.
// Gli elenchi vanno nel dopo dell'evento bozza_aperta (il giornale tiene la provenienza di ogni PB iniziale: D-PB03 A)
// e nella risposta di crea.

import (
	"context"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// presaInCarico: i passi 1 e 2 del commento di testa sullo snapshot della versione 1.
func presaInCarico(ctx context.Context, q *db.Queries, c Creazione3, in IngressoPartenza, s *Snapshot3, passo *passoStep,
	cp ConfermaDiPartenza) (PresaInCarico, error) {
	var out PresaInCarico
	per, err := perimetroDi(in)
	if err != nil {
		return out, err
	}
	if out.Conversioni, out.NonConvertite, err = convertiPreassegnazioni(ctx, q, c.ThreadID, c.UtenteID, c.Comune.ChiaveSalvataggio, s, per); err != nil {
		return out, err
	}
	for _, x := range out.Conversioni {
		if !x.PBEsistente {
			out.Iniziali = append(out.Iniziali, Preassociazione3{AllegatoID: x.AllegatoID, ChiaveBozza: x.ChiaveBozza, Provenienza: ProvenienzaLegacy})
		}
	}
	var reg *importazione.Registro
	switch {
	case passo != nil:
		r := passo.registro
		reg = &r
	case c.Crea.Partenza == PartenzaConferma && cp.ImportazioneID.Valid && s.Sorgente != nil:
		riga, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: cp.ImportazioneID.UUID, ThreadID: c.ThreadID})
		if err != nil {
			return out, err
		}
		r, _, err := RileggiImportazione(riga)
		if err != nil {
			return out, err
		}
		reg = &r
	}
	if reg != nil && s.Sorgente != nil {
		pb, proposte, err := PBInizialiDelServer(ctx, q, c.ThreadID, c.Comune.ProdottoID, s, reg, s.Sorgente.Sha256, uuid.Nil, 0, nil, c.Proposta.step())
		if err != nil {
			return out, err
		}
		out.Iniziali = append(out.Iniziali, pb...)
		out.Proposte = proposte
	}
	return out, nil
}

// PresaInCarico raccoglie quello che il server scrive alla nascita della bozza oltre alla versione 1: le PB iniziali
// (motore e legacy), i suggerimenti, le conversioni e le pre-assegnazioni non convertite (per la risposta e l'evento).
type PresaInCarico struct {
	Iniziali      []Preassociazione3
	Proposte      []PreassociazioneProposta3
	Conversioni   []ConversioneLegacy3
	NonConvertite []PreassegnazioneNonConvertita3
}

// nelDopo mette gli elenchi non vuoti nel dopo di un evento del server.
func (p PresaInCarico) nelDopo(dopo map[string]any) {
	if len(p.Iniziali) > 0 {
		dopo["preassociazioni_iniziali"] = p.Iniziali
	}
	if len(p.Proposte) > 0 {
		dopo["preassociazioni_proposte"] = p.Proposte
	}
	if len(p.Conversioni) > 0 {
		dopo["conversioni_legacy"] = p.Conversioni
	}
	if len(p.NonConvertite) > 0 {
		dopo["preassegnazioni_non_convertite"] = p.NonConvertite
	}
}

// elenchiDellaPresa: i nomi degli elenchi di PresaInCarico nel dopo, da ridurre per ultimi nei 128 KiB.
var elenchiDellaPresa = []string{"preassegnazioni_non_convertite", "preassociazioni_proposte", "conversioni_legacy", "preassociazioni_iniziali"}
