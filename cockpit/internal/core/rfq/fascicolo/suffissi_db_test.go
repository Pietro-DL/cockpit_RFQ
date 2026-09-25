//go:build integrazione

// L4 — Fascicolo v3: i suffissi decorativi del cliente nelle proposte di struttura di uno STEP, contro
// PostgreSQL vero. Un nodo «77720000_PRT» di un cliente che dichiara «_PRT» e' il pezzo 77720000; un
// componente accettato come «77720000_PRT» prima che la regola ci fosse e' lo stesso pezzo, e il nodo ne
// diventa un duplicato invece di un secondo componente. E D16: il codice del documento resta solo se E' un
// codice di famiglia, non se una famiglia ci trova dentro qualcosa. Per chi non dichiara suffissi non cambia
// niente. La regola pura sta in classificazione (suffissi_test.go).

package fascicolo_test

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

const regoleSuffissoPRT = `{"suffissi_decorativi": ["_PRT"]}`

// La famiglia dei disegni 777 senza revisione nel codice. La regex non chiude con \b: in «77720000_PRT»
// trova 77720000.
const (
	famiglia777Semplice      = `{"famiglie_codice": [{"regex": "(?P<codice>777\\d{5})", "descrizione": "disegni 777", "esempio": "77720000"}]`
	famiglia777ConSuffisso   = famiglia777Semplice + `, "suffissi_decorativi": ["_PRT"]}`
	famiglia777SenzaSuffissi = famiglia777Semplice + `}`
)

// conRegole scrive le regole del cliente della RFQ del banco.
func (b *banco) conRegole(regole string) {
	b.t.Helper()
	b.esegui(`UPDATE cliente SET regole = $1 FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $2`, regole, b.thread)
}

// letturaDeiNodi: "chiave=codice/rev/origine:stato" per ogni proposta di nodo della RFQ, in ordine di chiave.
func (b *banco) letturaDeiNodi() string {
	return uno[string](b, `SELECT coalesce(string_agg(chiave || '=' || coalesce(codice, '-') || '/' || coalesce(rev, '-') || '/' ||
		coalesce(origine_codice::text, '-') || ':' || stato, ' ' ORDER BY chiave), '') FROM componente_proposta WHERE thread_id = $1`, b.thread)
}

// nodiEComponenti: "chiave:stato:componente" per ogni proposta di nodo, in ordine di chiave.
func (b *banco) nodiEComponenti() string {
	return uno[string](b, `SELECT coalesce(string_agg(chiave || ':' || stato || ':' || coalesce(componente_id::text, '-'), ' ' ORDER BY chiave), '')
		FROM componente_proposta WHERE thread_id = $1`, b.thread)
}

// codiciDeiComponenti: i codici dei componenti della RFQ, in ordine.
func (b *banco) codiciDeiComponenti() string {
	return uno[string](b, `SELECT coalesce(string_agg(codice, ' ' ORDER BY codice), '') FROM componente WHERE thread_id = $1`, b.thread)
}

// stepConSuffissi: un assieme 77840000 con due figli scritti come li scrive il CAD del cliente.
func stepConSuffissi() fattiSTEP {
	return fattiSTEP{nodi: []string{"#1=77840000", "#2=77720000_PRT", "#3=77730000_C_PRT"}, archi: []string{"#1>#2*2", "#1>#3"}}
}

// Un nodo dello STEP che porta il suffisso decorativo del cliente propone il codice del pezzo: 77720000, non
// 77720000_PRT; con la revisione prima del suffisso, codice e revisione separati. Il grezzo del file resta
// com'e' (e' un fatto). Accettato, il componente nasce con il codice del pezzo.
func TestUnNodoConIlSuffissoDecorativoProponeIlCodiceDelPezzo(t *testing.T) {
	b := nuovoBanco(t)
	b.conRegole(regoleSuffissoPRT)
	a := b.allegatoStep("77840000.stp", strings.Repeat("d", 64))
	b.applica(a, stepConSuffissi().json())
	if got, want := b.letturaDeiNodi(), "#1=77840000/-/generico:aperta #2=77720000/-/generico:aperta #3=77730000/C/generico:aperta"; got != want {
		t.Fatalf("nodi = %q, attesi %q", got, want)
	}
	if got := uno[string](b, `SELECT nome_grezzo || '|' || id_grezzo FROM componente_proposta WHERE thread_id = $1 AND chiave = '#2'`, b.thread); got != "77720000_PRT|77720000_PRT" {
		t.Errorf("il grezzo del nodo e' cambiato: %q", got)
	}
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "77720000 entra nella BOM come sciolto") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.codiciDeiComponenti(); got != "77720000" {
		t.Errorf("componenti = %q, atteso il solo 77720000", got)
	}
}

// Un pezzo accettato come «77720000_PRT» prima che la regola ci fosse e' lo stesso pezzo che lo STEP, letto
// adesso con la regola, chiama 77720000: il nodo ne e' un duplicato, agganciato a lui, e l'arco uguale a quello
// della working pure. Niente da decidere, nessun secondo componente, e il codice del componente non cambia.
// Vale sia per il nodo scritto senza suffisso sia per quello scritto con.
func TestUnComponenteConIlSuffissoAccettatoPrimaDellaRegolaEDuplicatoDelNodo(t *testing.T) {
	for _, nodo := range []string{"77720000", "77720000_PRT"} {
		t.Run("nodo "+nodo, func(t *testing.T) {
			b := nuovoBanco(t)
			p := b.componente("77840000", db.TipoComponenteFinito)
			x := b.componente("77720000_PRT", db.TipoComponenteSciolto)
			b.arco(p, x, 2)
			b.conRegole(regoleSuffissoPRT)
			a := b.allegatoStep("77840000.stp", strings.Repeat("e", 64))
			es := b.applica(a, fattiSTEP{nodi: []string{"#1=77840000", "#2=" + nodo}, archi: []string{"#1>#2*2"}}.json())
			if got, want := b.nodiEComponenti(), "#1:duplicato:"+p.String()+" #2:duplicato:"+x.String(); got != want {
				t.Errorf("nodi = %q, attesi %q", got, want)
			}
			if got := uno[string](b, `SELECT codice FROM componente_proposta WHERE thread_id = $1 AND chiave = '#2'`, b.thread); got != "77720000" {
				t.Errorf("il nodo propone %q: il codice letto e' quello del pezzo, 77720000", got)
			}
			if got := b.relazioniProposte(); got != "#1>#2*2:duplicato" {
				t.Errorf("relazioni = %q, attesa #1>#2*2:duplicato", got)
			}
			if es.NodiAperti != 0 || es.RelazioniAperte != 0 {
				t.Errorf("proposte aperte: %d nodi e %d relazioni, attese nessuna", es.NodiAperti, es.RelazioniAperte)
			}
			if got := b.codiciDeiComponenti(); got != "77720000_PRT 77840000" {
				t.Errorf("componenti = %q: nessun componente nuovo, e il codice di quello vecchio non cambia", got)
			}
		})
	}
}

// Il controllo: senza la regola lo STEP si legge come prima. Il nodo tiene il suffisso nel codice, e 77720000
// non e' il componente «77720000_PRT»: e' un nodo nuovo, con il suo arco, da decidere.
func TestSenzaSuffissiLoStepSiLeggeComePrima(t *testing.T) {
	t.Run("i codici dei nodi", func(t *testing.T) {
		b := nuovoBanco(t)
		a := b.allegatoStep("77840000.stp", strings.Repeat("d", 64))
		b.applica(a, stepConSuffissi().json())
		if got, want := b.letturaDeiNodi(), "#1=77840000/-/generico:aperta #2=77720000_PRT/-/generico:aperta #3=77730000_C_PRT/-/generico:aperta"; got != want {
			t.Errorf("nodi = %q, attesi %q", got, want)
		}
	})
	t.Run("il componente con il suffisso", func(t *testing.T) {
		b := nuovoBanco(t)
		p := b.componente("77840000", db.TipoComponenteFinito)
		x := b.componente("77720000_PRT", db.TipoComponenteSciolto)
		b.arco(p, x, 2)
		a := b.allegatoStep("77840000.stp", strings.Repeat("e", 64))
		es := b.applica(a, fattiSTEP{nodi: []string{"#1=77840000", "#2=77720000"}, archi: []string{"#1>#2*2"}}.json())
		if got, want := b.nodiEComponenti(), "#1:duplicato:"+p.String()+" #2:aperta:-"; got != want {
			t.Errorf("nodi = %q, attesi %q", got, want)
		}
		if got := b.relazioniProposte(); got != "#1>#2*2:aperta" {
			t.Errorf("relazioni = %q, attesa #1>#2*2:aperta", got)
		}
		if es.NodiAperti != 1 || es.RelazioniAperte != 1 {
			t.Errorf("proposte aperte: %d nodi e %d relazioni, attesi 1 e 1", es.NodiAperti, es.RelazioniAperte)
		}
	})
}

// D16 con la regola nuova: il codice della proposta del documento resta solo se E' un codice di famiglia.
// «77720000_PRT» contiene il codice di famiglia 77720000 ma non lo e': la radice dello STEP lo corregge, con
// fonte regola_cliente e la famiglia nei dettagli. Non dipende dai suffissi: e' la regola di D16. Un codice che
// E' di famiglia resta, anche se la radice ne dice un altro.
func TestD16IlCodiceDelDocumentoRestaSoloSeEUnCodiceDiFamiglia(t *testing.T) {
	casi := []struct {
		nome, regole, codice, atteso string
	}{
		{"il codice con il suffisso, con la regola", famiglia777ConSuffisso, "77720000_PRT", "77720000:-:regola_cliente:disegni 777:id:77720000_PRT"},
		{"il codice con il suffisso, senza la regola", famiglia777SenzaSuffissi, "77720000_PRT", "77720000:-:regola_cliente:disegni 777:id:77720000_PRT"},
		{"un codice di famiglia", famiglia777ConSuffisso, "77722757", "77722757:-:nome_file:-:-:-"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			b := nuovoBanco(t)
			b.conRegole(c.regole)
			a := b.allegatoStep("assieme.stp", strings.Repeat("f", 64))
			b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte, stato)
				VALUES ($1, $2, 'cad_3d', $3, 40, 'nome_file', 'aperta')`, a.AllegatoID, b.thread, c.codice)
			b.applica(a, fattiSTEP{nodi: []string{"#1=77720000_PRT", "#2=77811111"}, archi: []string{"#1>#2"}}.json())
			got := uno[string](b, `SELECT codice || ':' || coalesce(rev, '-') || ':' || fonte || ':' || coalesce(dettagli ->> 'famiglia', '-') || ':' ||
				coalesce(dettagli ->> 'dove', '-') || ':' || coalesce(dettagli ->> 'testo', '-') FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID)
			if got != c.atteso {
				t.Errorf("proposta del documento = %q, attesa %q", got, c.atteso)
			}
			// la radice e' il pezzo di famiglia, con o senza la regola
			if got := b.letturaDeiNodi(); !strings.HasPrefix(got, "#1=77720000/-/famiglia:aperta") {
				t.Errorf("la radice dello STEP: %q", got)
			}
		})
	}
}

// Una famiglia con la revisione in coda non legge il suffisso come revisione: «77720000_PRT» e' 77720000 senza
// revisione, non 77720000 rev P; «77730000_C_PRT» e' 77730000 rev C.
func TestUnaFamigliaConLaRevisioneNonLeggeIlSuffissoComeRevisione(t *testing.T) {
	b := nuovoBanco(t)
	b.conRegole(`{"famiglie_codice": [{"regex": "(?P<codice>777\\d{5})(?:_(?P<rev>[A-Z]))?", "rev_nel_codice": true, "descrizione": "disegni 777",
		"esempio": "77722757_B"}], "suffissi_decorativi": ["_PRT"]}`)
	a := b.allegatoStep("77740000.stp", strings.Repeat("a", 64))
	b.applica(a, fattiSTEP{nodi: []string{"#1=77740000", "#2=77720000_PRT", "#3=77730000_C_PRT"}, archi: []string{"#1>#2", "#1>#3"}}.json())
	if got, want := b.letturaDeiNodi(), "#1=77740000/-/famiglia:aperta #2=77720000/-/famiglia:aperta #3=77730000/C/famiglia:aperta"; got != want {
		t.Errorf("nodi = %q, attesi %q", got, want)
	}
}

// Il pezzo nato come «77720000_PRT» e poi archiviato: il nodo «77720000» dice che accettarlo lo ripristina, e
// accettarlo ripristina proprio quello (nessun secondo componente 77720000).
func TestAccettareIlNodoRipristinaIlPezzoConIlSuffisso(t *testing.T) {
	b := nuovoBanco(t)
	p := b.componente("77840000", db.TipoComponenteFinito)
	x := b.componente("77720000_PRT", db.TipoComponenteSciolto)
	b.esegui(`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'tolto dal cliente' WHERE componente_id = $1`, x, b.utente)
	b.conRegole(regoleSuffissoPRT)
	a := b.allegatoStep("77840000.stp", strings.Repeat("b", 64))
	b.applica(a, fattiSTEP{nodi: []string{"#1=77840000", "#2=77720000"}, archi: []string{"#1>#2*2"}}.json())
	if got := uno[string](b, `SELECT stato || ':' || coalesce(nota, '') FROM componente_proposta WHERE thread_id = $1 AND chiave = '#2'`, b.thread); !strings.HasPrefix(got, "aperta:") ||
		!strings.Contains(got, "77720000_PRT") {
		t.Fatalf("il nodo davanti al pezzo archiviato: %q", got)
	}
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "77720000_PRT era archiviato: ripristinato") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.codiciDeiComponenti(); got != "77720000_PRT 77840000" {
		t.Errorf("componenti = %q: il pezzo si ripristina, non ne nasce un altro", got)
	}
	if got := uno[string](b, `SELECT (archiviato_il IS NULL)::text FROM componente WHERE componente_id = $1`, x); got != "true" {
		t.Errorf("il pezzo e' ancora archiviato")
	}
	if got := b.nodiEComponenti(); !strings.Contains(got, "#2:duplicato:"+x.String()) {
		t.Errorf("il nodo e' un duplicato del pezzo: %q", got)
	}
	_ = p
}

// Un file «77720000» arriva in una RFQ in cui il pezzo e' nato come «77720000_PRT» prima della regola: il piano
// non chiede di aggiungere 77720000 (sarebbe un secondo componente), dice che e' lo stesso pezzo e offre di
// assegnarlo a quello.
func TestIlPianoRitrovaIlPezzoConIlSuffisso(t *testing.T) {
	b := nuovoBanco(t)
	x := b.componente("77720000_PRT", db.TipoComponenteSciolto)
	b.conRegole(regoleSuffissoPRT)
	msg := b.messaggioOutlook()
	pdf, _ := b.allegatoInStaging(msg, 1, "77720000_PRT.pdf", "%PDF disegno")
	b.esegui(`UPDATE allegato SET stato = 'analizzato' WHERE allegato_id = $1`, pdf)
	b.esegui(`UPDATE documento_proposta SET tipo_proposto = 'disegno_2d', codice = '77720000', fonte = 'cartiglio', confidenza = 95 WHERE allegato_id = $1`, pdf)
	var p fascicolo.PianoFascicolo
	if err := b.tx(func(q *db.Queries) (err error) {
		p, err = fascicolo.LeggiPianoFascicolo(b.ctx, q, b.thread)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(p.File) != 1 {
		t.Fatalf("piano: %+v", p.File)
	}
	v := p.File[0]
	if v.Stato != fascicolo.VoceDecidere || v.Alias == nil || v.Alias.ComponenteID != x || !strings.Contains(v.Domande[0].Testo, "lo stesso pezzo con il suffisso del cliente") {
		t.Errorf("la voce del file: stato %s, alias %+v, domande %+v", v.Stato, v.Alias, v.Domande)
	}
	// senza la regola il pezzo non si ritrova: 77720000 non e' nella BOM
	b.conRegole(`{}`)
	if err := b.tx(func(q *db.Queries) (err error) {
		p, err = fascicolo.LeggiPianoFascicolo(b.ctx, q, b.thread)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if v := p.File[0]; v.Alias != nil || !strings.Contains(v.Domande[0].Testo, "non è nella BOM") {
		t.Errorf("senza la regola: alias %+v, domande %+v", v.Alias, v.Domande)
	}
}
