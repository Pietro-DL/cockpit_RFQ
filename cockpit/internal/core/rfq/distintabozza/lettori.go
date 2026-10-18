package distintabozza

// I lettori dei formati salvati (EM §2.5, §9.1, §9.7). Si scrive solo il formato 3; si rileggono:
//   - le versioni della bozza nei formati 2 (il lettore di snapshot.go, immutato) e 3;
//   - gli snapshot delle conferme nei formati 1 (la forma del contratto rev. 3 §5.6, letta com'e'), 2 e 3. Per lo
//     storico (R7) un contenuto che il lettore del suo formato non sa leggere non si nasconde: si mostra con i campi
//     grezzi e contenuto_non_leggibile.
// Qui anche i pezzi della testata che R1 restituisce nel formato 3 (formato, vocabolario, chiave dell'ultimo
// salvataggio, registro della versione) e il passaggio fra il DTO del registro e le righe di distinta_importazione.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// FormatoSnapshot1 e' il formato delle conferme di «Conferma l'albero» del giro 4 (contratto rev. 3 §5.6): si legge
// soltanto.
const FormatoSnapshot1 = 1

// SnapshotLetto e' la vista comune di uno snapshot salvato, in uno dei tre formati.
type SnapshotLetto struct {
	Formato int
	// Formato1: lo snapshot di una conferma formato 1 ({formato: 1, perimetro}), canonico: la sua forma interna non e'
	// tipizzata (contratto rev. 3 §5.6 non la fissa campo per campo).
	Formato1 json.RawMessage
	Formato2 *Snapshot
	Formato3 *Snapshot3
	// Canonico: i byte canonici dello snapshot (per un contenuto non leggibile, il grezzo ricanonicalizzato quando si
	// puo', altrimenti com'era).
	Canonico json.RawMessage
	// Impronta: l'impronta registrata.
	Impronta string
	// NonLeggibile e Motivo: il lettore del formato non sa leggere il contenuto, o l'impronta non torna (R7:
	// contenuto_non_leggibile, i campi grezzi accanto).
	NonLeggibile bool
	Motivo       string
}

// formatoDichiarato legge il campo formato di uno snapshot, senza altro.
func formatoDichiarato(raw json.RawMessage) (int, bool) {
	var t struct {
		Formato *int `json:"formato"`
	}
	if json.Unmarshal(raw, &t) != nil || t.Formato == nil {
		return 0, false
	}
	return *t.Formato, true
}

// RileggiVersione rilegge lo snapshot di una versione della bozza con il lettore del formato della testata (2 o 3) e
// ne ricontrolla l'impronta. Uno snapshot di un altro formato, illeggibile o con un'impronta diversa e' un guasto dei
// dati salvati (ErrSnapshotIncoerente), come per il formato 2.
func RileggiVersione(formatoTestata int, raw json.RawMessage, impronta string) (SnapshotLetto, error) {
	if f, ok := formatoDichiarato(raw); !ok || f != formatoTestata {
		return SnapshotLetto{}, fmt.Errorf("%w: la versione non e' del formato %d della bozza", ErrSnapshotIncoerente, formatoTestata)
	}
	switch formatoTestata {
	case FormatoSnapshot:
		s, imp, err := rileggiSnapshot(raw, impronta)
		if err != nil {
			return SnapshotLetto{}, err
		}
		can, err := s.Canonico()
		if err != nil {
			return SnapshotLetto{}, fmt.Errorf("%w: %v", ErrSnapshotIncoerente, err)
		}
		return SnapshotLetto{Formato: FormatoSnapshot, Formato2: &s, Canonico: can, Impronta: imp}, nil
	case FormatoSnapshot3:
		s, imp, err := rileggiSnapshot3(raw, impronta)
		if err != nil {
			return SnapshotLetto{}, err
		}
		can, err := s.codifica()
		if err != nil {
			return SnapshotLetto{}, fmt.Errorf("%w: %v", ErrSnapshotIncoerente, err)
		}
		return SnapshotLetto{Formato: FormatoSnapshot3, Formato3: &s, Canonico: can, Impronta: imp}, nil
	}
	return SnapshotLetto{}, fmt.Errorf("%w: una bozza del formato %d non si rilegge (solo 2 e 3)", ErrSnapshotIncoerente, formatoTestata)
}

// rileggiSnapshot3 rilegge uno snapshot formato 3 salvato e ne ricontrolla l'impronta: la forma canonica rifatta deve
// dare l'impronta registrata (il lettore normalizza esattamente come lo scrittore, Snapshot3.Canonico: EM §3.10).
func rileggiSnapshot3(raw json.RawMessage, impronta string) (Snapshot3, string, error) {
	s, e := leggiSnapshot3(raw, 0)
	if e != nil {
		return Snapshot3{}, "", fmt.Errorf("%w: non si rilegge (%v)", ErrSnapshotIncoerente, e)
	}
	can, err := s.codifica()
	if err != nil {
		return Snapshot3{}, "", fmt.Errorf("%w: %v", ErrSnapshotIncoerente, err)
	}
	imp := jsoncanonico.Impronta(can)
	if imp != strings.TrimSpace(impronta) {
		return Snapshot3{}, "", fmt.Errorf("%w: impronta %s, registrata %s", ErrSnapshotIncoerente, imp, impronta)
	}
	return s, imp, nil
}

// LeggiSnapshotConferma legge lo snapshot di una conferma nel suo formato (1, 2 o 3) per lo storico. Non fallisce mai:
// un contenuto che il lettore non sa leggere, o con un'impronta diversa da quella registrata, torna con NonLeggibile e i
// byte grezzi (EM §9.7: «mai nascosto»).
func LeggiSnapshotConferma(formato int, raw json.RawMessage, impronta string) SnapshotLetto {
	out := SnapshotLetto{Formato: formato, Impronta: strings.TrimSpace(impronta)}
	nonLeggibile := func(motivo string) SnapshotLetto {
		out.NonLeggibile, out.Motivo = true, motivo
		if can, err := jsoncanonico.Codifica(raw); err == nil {
			out.Canonico = can
		} else {
			out.Canonico = append(json.RawMessage(nil), raw...)
		}
		return out
	}
	if f, ok := formatoDichiarato(raw); !ok || f != formato {
		return nonLeggibile(fmt.Sprintf("lo snapshot non dichiara il formato %d della conferma", formato))
	}
	switch formato {
	case FormatoSnapshot1:
		can, err := leggiSnapshot1(raw)
		if err != nil {
			return nonLeggibile(err.Error())
		}
		if jsoncanonico.Impronta(can) != out.Impronta {
			return nonLeggibile("l'impronta dello snapshot formato 1 non e' quella registrata")
		}
		out.Formato1, out.Canonico = can, can
	case FormatoSnapshot:
		s, _, err := rileggiSnapshot(raw, impronta)
		if err != nil {
			return nonLeggibile(err.Error())
		}
		can, err := s.Canonico()
		if err != nil {
			return nonLeggibile(err.Error())
		}
		out.Formato2, out.Canonico = &s, can
	case FormatoSnapshot3:
		s, _, err := rileggiSnapshot3(raw, impronta)
		if err != nil {
			return nonLeggibile(err.Error())
		}
		can, err := s.codifica()
		if err != nil {
			return nonLeggibile(err.Error())
		}
		out.Formato3, out.Canonico = &s, can
	default:
		return nonLeggibile(fmt.Sprintf("formato %d sconosciuto", formato))
	}
	return out
}

// leggiSnapshot1: lo snapshot formato 1, {formato: 1, perimetro: {…}}, nella sua forma canonica. Il perimetro resta
// com'e': il lettore controlla solo i due campi di testa.
func leggiSnapshot1(raw json.RawMessage) (json.RawMessage, error) {
	can, err := jsoncanonico.Codifica(raw)
	if err != nil {
		return nil, fmt.Errorf("lo snapshot formato 1 non si legge: %v", err)
	}
	var campi map[string]json.RawMessage
	if err := json.Unmarshal(can, &campi); err != nil {
		return nil, errors.New("lo snapshot formato 1 non e' un oggetto")
	}
	for k := range campi {
		if k != "formato" && k != "perimetro" {
			return nil, fmt.Errorf("lo snapshot formato 1 ha il campo %q, fuori dalla sua forma", k)
		}
	}
	if p := strings.TrimSpace(string(campi["perimetro"])); !strings.HasPrefix(p, "{") {
		return nil, errors.New("lo snapshot formato 1 non ha il perimetro")
	}
	return can, nil
}

// ------------------------------------------------------------------ la testata nel formato 3 (R1)

// VocabolarioDelFormato: il vocabolario del giornale di una bozza del formato dato (EM §2.5): 1 per il formato 2, 2 per
// il formato 3.
func VocabolarioDelFormato(formato int) int {
	if formato == FormatoSnapshot3 {
		return VocabolarioFormato3
	}
	return Vocabolario
}

// TestataFormato3 sono i campi che R1 aggiunge nel formato 3 (EM §9.1): formato e vocabolario espliciti, la chiave
// dell'ultimo salvataggio, il registro della versione e i registri citati dalle decisioni.
type TestataFormato3 struct {
	Formato                 int                `json:"formato"`
	Vocabolario             int                `json:"vocabolario"`
	ChiaveUltimoSalvataggio *uuid.UUID         `json:"chiave_ultimo_salvataggio,omitempty"`
	Importazione            *ImportazioneLetta `json:"importazione,omitempty"`
	ImportazioniCitate      []RegistroCitato   `json:"importazioni_citate,omitempty"`
}

// ImportazioneLetta e' il blocco importazione di R1: la testata del registro, le sue regole e, con ?importazione=1, il
// contenuto.
type ImportazioneLetta struct {
	ImportazioneID    uuid.UUID       `json:"importazione_id"`
	Impronta          string          `json:"impronta"`
	ImprontaContenuto string          `json:"impronta_contenuto"`
	ImprontaEvidenze  string          `json:"impronta_evidenze"`
	ImprontaRegole    string          `json:"impronta_regole"`
	Gruppo            string          `json:"gruppo"`
	CreataDa          uuid.UUID       `json:"creata_da"`
	CreataIl          time.Time       `json:"creata_il"`
	Regole            json.RawMessage `json:"regole"`
	Contenuto         json.RawMessage `json:"contenuto,omitempty"`
}

// RegistroCitato e' un registro citato da una decisione della bozza (importazioni_citate di R1).
type RegistroCitato struct {
	Impronta string    `json:"impronta"`
	CreataIl time.Time `json:"creata_il"`
}

// ImportazioneLettaDa compone il blocco importazione da un registro riletto e dalla sua testata.
func ImportazioneLettaDa(r importazione.Registro, t importazione.Testata, conContenuto bool) ImportazioneLetta {
	out := ImportazioneLetta{ImportazioneID: t.ImportazioneID, Impronta: r.Impronta, ImprontaContenuto: r.Contenuto.ImprontaContenuto,
		ImprontaEvidenze: r.Contenuto.ImprontaEvidenze, ImprontaRegole: r.Contenuto.ImprontaRegole, Gruppo: r.Contenuto.Gruppo.Chiave,
		CreataDa: t.CreataDa, CreataIl: t.CreataIl, Regole: r.Grezzo("regole")}
	if conContenuto {
		out.Contenuto = append(json.RawMessage(nil), r.Canonico...)
	}
	return out
}

// ------------------------------------------------------------------ il registro e le righe di distinta_importazione

// TestataImportazione: le colonne di una riga di distinta_importazione nella testata del DTO.
func TestataImportazione(r db.GetDistintaImportazioneRow) importazione.Testata {
	t := importazione.Testata{ImportazioneID: r.ImportazioneID, ThreadID: r.ThreadID, ProdottoID: r.ProdottoID,
		Impronta: strings.TrimSpace(r.Impronta), ImprontaContenuto: strings.TrimSpace(r.ImprontaContenuto),
		ImprontaEvidenze: strings.TrimSpace(r.ImprontaEvidenze), ImprontaRegole: strings.TrimSpace(r.ImprontaRegole),
		FormatoRegistro: int(r.FormatoRegistro), VersioneImportazione: r.VersioneImportazione, Gruppo: r.Gruppo,
		FonteSha256: strings.TrimSpace(r.FonteSha256.String), FonteRadice: r.FonteRadice.String,
		AllegatiCitati: r.AllegatiCitati, CreataDa: r.CreataDa, CreataIl: r.CreataIl}
	if r.FonteAllegatoID.Valid {
		u := r.FonteAllegatoID.UUID
		t.FonteAllegatoID = &u
	}
	return t
}

// RileggiImportazione rilegge il registro di una riga e lo confronta con le sue colonne; un difetto e'
// importazione.ErrRegistroIncoerente.
func RileggiImportazione(r db.GetDistintaImportazioneRow) (importazione.Registro, importazione.Testata, error) {
	t := TestataImportazione(r)
	reg, err := importazione.Rileggi(t, r.Contenuto)
	return reg, t, err
}

// ParametriImportazione: i parametri dell'INSERT di un registro composto (importazione.Componi), con il suo autore. Le
// colonne vengono tutte dal contenuto: il CHECK ck_dimp_contenuto e i trigger della 0023 le ricontrollano.
func ParametriImportazione(r importazione.Registro, autore uuid.UUID) db.InsertDistintaImportazioneParams {
	t := r.Colonne()
	p := db.InsertDistintaImportazioneParams{ThreadID: t.ThreadID, ProdottoID: t.ProdottoID, Impronta: t.Impronta,
		ImprontaContenuto: t.ImprontaContenuto, ImprontaEvidenze: t.ImprontaEvidenze, ImprontaRegole: t.ImprontaRegole,
		FormatoRegistro: int16(t.FormatoRegistro), VersioneImportazione: t.VersioneImportazione, Gruppo: t.Gruppo,
		FonteSha256: testo(t.FonteSha256), FonteRadice: testo(t.FonteRadice), AllegatiCitati: t.AllegatiCitati,
		Contenuto: append(json.RawMessage(nil), r.Canonico...), CreataDa: autore}
	if p.AllegatiCitati == nil {
		p.AllegatiCitati = []uuid.UUID{}
	}
	p.FonteAllegatoID = nullUUID(t.FonteAllegatoID)
	return p
}
