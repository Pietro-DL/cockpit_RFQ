package valutazione

import (
	"sort"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// Ruolo e categoria di un componente (B5, fase 3; emendamento E1 §6, E1R §4.2; contratto §2.5, §2.6; T-E1-18,
// T-E1R-01, LD-19). Sono due cose: il ruolo dice dove sta il pezzo (prodotto o componente), la categoria che cosa è
// (fabbricato, commerciale, minuteria). La categoria è confermata solo da una ConfermaCategoria, che in A1c nessun
// adattatore produce: il DB non ha la categoria (LD-19), e il tipo commerciale del legacy non è la minuteria. Quella
// della grammatica va in Proposta e non esenta mai niente.

// I valori di Classificazione.Ruolo (emendamento E1 §6.1): prodotto per il componente del target e per il finito,
// componente per gli altri pezzi.
const (
	RuoloProdotto   = "prodotto"
	RuoloComponente = "componente"
)

// I valori di Classificazione.Categoria (contratto §2.5: un'estensione dichiarata delle categorie documentate, che sono
// solo {minuteria}; emendamento E1 §6.3).
const (
	CategoriaFabbricato     = "fabbricato"
	CategoriaCommerciale    = "commerciale"
	CategoriaMinuteria      = "minuteria"
	CategoriaNonDeterminata = "non_determinata"
)

// I valori di Classificazione.Origine (contratto §2.5): confermato (una ConfermaCategoria, o il tipo commerciale, che
// scrive solo una persona: emendamento E1 §6.2 e §6.3), derivato_dal_tipo (fabbricato dal tipo confermato), proposto
// (un nodo senza tipo deciso).
const (
	OrigineCategoriaConfermata = "confermato"
	OrigineCategoriaDerivata   = "derivato_dal_tipo"
	OrigineCategoriaProposta   = "proposto"
)

// I motivi di una classificazione (Classificazione.Motivo; il contratto non li fissa: scelte della fase 3, dubbio
// T-B5-51):
//   - finito_chiede_sempre_il_2d: il finito con una ConfermaCategoria minuteria: la categoria confermata si mostra, ma il
//     2D resta richiesto (R99 A; E1R §4.1);
//   - conferma_non_ammessa: una ConfermaCategoria con una categoria fuori da fabbricato, commerciale, minuteria: non
//     conta, e la categoria resta quella del tipo;
//   - tipo_non_determinato: un nodo senza tipo deciso, o un tipo fuori dall'enum del DB.
const (
	MotivoCategoriaFinitoSempre2D     = "finito_chiede_sempre_il_2d"
	MotivoCategoriaConfermaNonAmmessa = "conferma_non_ammessa"
	MotivoCategoriaTipoNonDeterminato = "tipo_non_determinato"
)

// I tipi di componente del DB (0001, tipo_componente), ripetuti qui perché il legacy non li esporta.
const (
	tipoSottoassieme = "sottoassieme"
	tipoSciolto      = "sciolto"
	tipoCommerciale  = "commerciale"
)

// Classificazione: il ruolo e la categoria di un componente (contratto §2.5, §2.6; T-E1-18, T-E1R-01).
//   - Ruolo: prodotto o componente.
//   - Categoria: quella confermata (ConfermaCategoria), o quella che deriva dal tipo confermato: fabbricato per finito,
//     sottoassieme e sciolto, commerciale per il commerciale, non_determinata per un nodo senza tipo deciso. È minuteria
//     solo con una conferma.
//   - Origine, Confermata: da dove viene la categoria, e se una persona l'ha confermata.
//   - Motivo: MotivoCategoria*, quando serve.
//   - Proposta: la categoria della grammatica, quando non coincide (uno sciolto o un commerciale della famiglia
//     minuteria hanno Proposta = minuteria): la nota «minuteria proposta, da confermare». Non esenta mai.
//
// L'esenzione dal 2D vale solo con Categoria = minuteria e Confermata, e mai per il prodotto (EsenteDal2D).
type Classificazione struct {
	Ruolo      string `json:"ruolo"`
	Categoria  string `json:"categoria"`
	Origine    string `json:"origine"`
	Confermata bool   `json:"confermata"`
	Motivo     string `json:"motivo,omitempty"`
	Proposta   string `json:"proposta,omitempty"`
}

// ConfermaCategoria: la categoria di un componente confermata da una persona, l'ingresso astratto (contratto §2.6):
// l'unico modo di avere una minuteria confermata. Mai un'esenzione per il finito. In A1c nessun adattatore la produce
// (LD-19): la regola si prova con conferme sintetiche.
type ConfermaCategoria struct {
	ComponenteID uuid.UUID `json:"componente_id"`
	Categoria    string    `json:"categoria"`
	Da           uuid.UUID `json:"da"`
	Il           time.Time `json:"il"`
}

// ComponenteDaClassificare: ciò che serve alla classificazione di un pezzo, l'ingresso astratto di Classifica.
//   - ComponenteID: il componente confermato; nil per un nodo proposto (che non ha una conferma).
//   - Tipo: il tipo confermato del componente (tipo_componente); "" per un nodo proposto.
//   - DelProdotto: il componente di un prodotto target. Il ruolo prodotto viene da qui, non solo dal tipo: il componente
//     di un target può non essere un finito (R99 A; R-23 della revisione della fase 3).
//   - Categorie: le categorie della grammatica sulla lettura del codice (motorea.LetturaForma.Categorie).
type ComponenteDaClassificare struct {
	ComponenteID *uuid.UUID `json:"componente_id,omitempty"`
	Tipo         string     `json:"tipo,omitempty"`
	DelProdotto  bool       `json:"del_prodotto,omitempty"`
	Categorie    []string   `json:"categorie,omitempty"`
}

// Classifica: la regola della classificazione (emendamento E1 §6.3, E1R §4.2; T-E1-18, T-E1R-01). È pura.
//  1. Il ruolo: prodotto per il componente di un prodotto target, qualunque sia il suo tipo (R-23), e per il finito
//     anche quando non è un target (emendamento E1 §6.3); componente per ogni altro pezzo (anche un nodo proposto).
//  2. La categoria dal tipo confermato: fabbricato (origine derivato_dal_tipo) per finito, sottoassieme e sciolto;
//     commerciale (origine confermato: il tipo commerciale lo scrive solo una persona) per il commerciale; un nodo senza
//     tipo deciso, o un tipo fuori dall'enum, non_determinata (origine proposto).
//  3. Una ConfermaCategoria sul componente (la più recente: il più grande «il», a parità il «da» minore, poi la
//     categoria) sostituisce la categoria, con l'origine confermato; una categoria fuori da fabbricato, commerciale,
//     minuteria non conta (conferma_non_ammessa). Sul prodotto (il finito o il componente del target) la minuteria
//     confermata si mostra, ma il 2D resta (finito_chiede_sempre_il_2d).
//  4. La proposta: minuteria, se la grammatica la dà e la categoria non è già minuteria. Non esenta mai.
func Classifica(c ComponenteDaClassificare, conferme []ConfermaCategoria) Classificazione {
	out := Classificazione{Ruolo: RuoloComponente}
	switch c.Tipo {
	case tipoFinito:
		out.Ruolo, out.Categoria, out.Origine = RuoloProdotto, CategoriaFabbricato, OrigineCategoriaDerivata
	case tipoSottoassieme, tipoSciolto:
		out.Categoria, out.Origine = CategoriaFabbricato, OrigineCategoriaDerivata
	case tipoCommerciale:
		out.Categoria, out.Origine, out.Confermata = CategoriaCommerciale, OrigineCategoriaConfermata, true
	default:
		out.Categoria, out.Origine, out.Motivo = CategoriaNonDeterminata, OrigineCategoriaProposta, MotivoCategoriaTipoNonDeterminato
	}
	if c.DelProdotto {
		out.Ruolo = RuoloProdotto
	}
	if k := confermaDel(c.ComponenteID, conferme); k != nil {
		switch k.Categoria {
		case CategoriaFabbricato, CategoriaCommerciale, CategoriaMinuteria:
			out.Categoria, out.Origine, out.Confermata, out.Motivo = k.Categoria, OrigineCategoriaConfermata, true, ""
			if out.Ruolo == RuoloProdotto && k.Categoria == CategoriaMinuteria {
				out.Motivo = MotivoCategoriaFinitoSempre2D
			}
		default:
			out.Motivo = MotivoCategoriaConfermaNonAmmessa
		}
	}
	for _, x := range c.Categorie {
		if x == string(grammatica.CategoriaMinuteria) && out.Categoria != CategoriaMinuteria {
			out.Proposta = CategoriaMinuteria
		}
	}
	return out
}

// EsenteDal2D: la sola esenzione dal 2D (R103 C; T-E1-18, T-E1R-01): la minuteria confermata, mai il prodotto (il
// finito o il componente del target: R99 A, R-23). Esenta solo il 2D: gli altri fabbisogni bloccanti del tipo restano.
// È l'unico controllo dell'esenzione: chi la usa (B6 nei nodi della BOM) chiama questa funzione, mai
// Categoria == minuteria && Confermata da solo.
func EsenteDal2D(c Classificazione) bool {
	return c.Ruolo != RuoloProdotto && c.Categoria == CategoriaMinuteria && c.Confermata
}

// confermaDel: la conferma più recente del componente (il più grande «il», a parità il «da» minore); nil senza.
func confermaDel(componente *uuid.UUID, conferme []ConfermaCategoria) *ConfermaCategoria {
	if componente == nil {
		return nil
	}
	var sue []ConfermaCategoria
	for _, k := range conferme {
		if k.ComponenteID == *componente {
			sue = append(sue, k)
		}
	}
	if len(sue) == 0 {
		return nil
	}
	sort.SliceStable(sue, func(i, j int) bool {
		if !sue[i].Il.Equal(sue[j].Il) {
			return sue[i].Il.After(sue[j].Il)
		}
		if sue[i].Da != sue[j].Da {
			return sue[i].Da.String() < sue[j].Da.String()
		}
		return sue[i].Categoria < sue[j].Categoria
	})
	return &sue[0]
}

// categorieDi: le categorie della grammatica come testo, in ordine, senza doppioni.
func categorieDi(c []grammatica.Categoria) []string {
	var out []string
	for _, x := range c {
		out = append(out, string(x))
	}
	return ordinatiUnici(out)
}
