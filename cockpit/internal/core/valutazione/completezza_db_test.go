//go:build integrazione

// L4 — la completezza documentale contro la vista, sul database di prova (contratto di A1c, §7, famiglia B5; T-E1R-03, la
// L4 di equivalenza; PO-33 come L4 sintetica; R62 d C, R82, R99 A, R103 C, K-01 con la lettura A): tre RFQ ACME scritte
// con SQL, due con un cliente che ha le sue regole dei fabbisogni e una con le sole regole di default, ognuna con un
// finito manuale, la sua BOM confermata (sottoassieme, sciolto, commerciale), e nella prima uno sciolto fuori dal
// perimetro e un componente archiviato; lette con caricatore.Carica in sola lettura, poi ValutaProdotti.
//   - L'insieme (componente, tipo di documento, bloccante) dei fabbisogni di A1c, voci e non bloccanti, è quello di
//     v_fascicolo, con le esclusioni nominate: E1 le voci che la vista non ha (schema_senza_2d, regola_cliente_senza_2d);
//     E2 il 2D del finito con regola_cliente_2d_non_bloccante, dalle due parti; E3 le righe della vista dei componenti
//     fuori dal perimetro di ogni target, con a parte la prova che ogni componente attivo raggiungibile dal componente di
//     un target compare in A1c; E4 le righe con una rimozione aperta e E5 la minuteria confermata non ci sono nella
//     fixture (nessuna rimozione, nessuna ConfermaCategoria: LD-19). La nota di K-01 A su un componente coincide con la
//     vista e si confronta.
//   - Per ogni componente confermato, la risoluzione delle regole che Go usa per i nodi proposti (FabbisogniDelTipo sui
//     fabbisogni effettivi di ListFabbisognoEffettivo) dà le stesse righe della vista (decisioni sull'analista, punto 1):
//     così la prova non è una tautologia della base presa dalla vista.
//   - PO-33: le note delle regole del cliente sui finiti, sugli sciolti, sui commerciali e sui sottoassiemi.
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.

package valutazione_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/testutil"
)

// scenaEquivalenza: le tre RFQ e i loro componenti, per nome: F il finito, S il sottoassieme, L lo sciolto, C il
// commerciale, O lo sciolto fuori dal perimetro, R l'archiviato; la lettera della RFQ dopo (FA, SB, …).
type scenaEquivalenza struct {
	operatore uuid.UUID
	thread    map[string]uuid.UUID
	comp      map[string]uuid.UUID
	deroghe   map[string]uuid.UUID
}

// costruisciScenaEquivalenza scrive le tre RFQ:
//   - A, cliente ACME-A: regole proprie per il finito (solo lo sviluppo_dxf non bloccante: niente 2D né cad_3d), per lo
//     sciolto (2D non bloccante, cad_3d bloccante) e per il commerciale (2D non bloccante); il sottoassieme con le
//     regole di default. Finito 7120100, sottoassieme 7120101 sotto il finito, sciolto 7120102 sotto il sottoassieme,
//     commerciale 7120103 sotto il finito, sciolto 7120104 senza relazione, sciolto 7120105 archiviato sotto il finito;
//   - B, cliente ACME-B: regole proprie per il finito (cad_3d bloccante, 2D non bloccante), per lo sciolto (solo il
//     cad_3d bloccante), per il commerciale (solo il cad_3d non bloccante) e per il sottoassieme (2D non bloccante).
//     Finito 7120200 con sciolto 7120201, commerciale 7120202, sottoassieme 7120203;
//   - C, cliente ACME-C senza regole proprie: finito 7120300 con sciolto 7120301 e commerciale 7120302; una deroga sul
//     cad_3d del finito e una sul 2D dello sciolto.
func costruisciScenaEquivalenza(t *testing.T, p *pgxpool.Pool) scenaEquivalenza {
	t.Helper()
	ctx := context.Background()
	riga := func(dst any, sql string, arg ...any) {
		t.Helper()
		if err := p.QueryRow(ctx, sql, arg...).Scan(dst); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	esegui := func(sql string, arg ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, arg...); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	s := scenaEquivalenza{thread: map[string]uuid.UUID{}, comp: map[string]uuid.UUID{}, deroghe: map[string]uuid.UUID{}}
	riga(&s.operatore, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('AC', 'Prova ACME', 'Tecnico') RETURNING utente_id`)
	esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)`, versioneAnalizzatore, hashConfigurazione)
	clienti := map[string]uuid.UUID{}
	for _, x := range []string{"A", "B", "C"} {
		var c, th uuid.UUID
		riga(&c, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1, $2) RETURNING cliente_id`, "ACME-"+x, "ACME "+x+" S.p.A.")
		riga(&th, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa, oggetto, creato_da)
			VALUES ($1, 'outlook', now() - interval '3 hours', $2, $3, $4) RETURNING thread_id`, c, `ACME\WIP\rfq-eq-`+x, "RFQ ACME26-07"+x, s.operatore)
		clienti[x], s.thread[x] = c, th
	}
	regola := func(cliente, tipoComp, tipoDoc string, bloccante bool) {
		t.Helper()
		esegui(`INSERT INTO fabbisogno_documento (cliente_id, tipo_componente, tipo, bloccante) VALUES ($1, $2, $3, $4)`,
			clienti[cliente], tipoComp, tipoDoc, bloccante)
	}
	regola("A", "finito", "sviluppo_dxf", false)
	regola("A", "sciolto", "disegno_2d", false)
	regola("A", "sciolto", "cad_3d", true)
	regola("A", "commerciale", "disegno_2d", false)
	regola("B", "finito", "cad_3d", true)
	regola("B", "finito", "disegno_2d", false)
	regola("B", "sciolto", "cad_3d", true)
	regola("B", "commerciale", "cad_3d", false)
	regola("B", "sottoassieme", "disegno_2d", false)

	componente := func(nome, thread, codice, tipo, origine string) {
		t.Helper()
		var id uuid.UUID
		riga(&id, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, $3, $4, $5) RETURNING componente_id`,
			s.thread[thread], codice, tipo, origine, s.operatore)
		s.comp[nome] = id
	}
	relazione := func(thread, padre, figlio string, qta int) {
		t.Helper()
		esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, $4, 'manuale', $5)`,
			s.thread[thread], s.comp[padre], s.comp[figlio], qta, s.operatore)
	}
	componente("FA", "A", "7120100", "finito", "manuale")
	componente("SA", "A", "7120101", "sottoassieme", "manuale")
	componente("LA", "A", "7120102", "sciolto", "manuale")
	componente("CA", "A", "7120103", "commerciale", "manuale")
	componente("OA", "A", "7120104", "sciolto", "manuale")
	componente("RA", "A", "7120105", "sciolto", "manuale")
	relazione("A", "FA", "SA", 1)
	relazione("A", "SA", "LA", 2)
	relazione("A", "FA", "CA", 4)
	relazione("A", "FA", "RA", 1)
	esegui(`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'tolto dalla BOM' WHERE componente_id = $1`,
		s.comp["RA"], s.operatore)
	componente("FB", "B", "7120200", "finito", "manuale")
	componente("LB", "B", "7120201", "sciolto", "manuale")
	componente("CB", "B", "7120202", "commerciale", "manuale")
	componente("SB", "B", "7120203", "sottoassieme", "manuale")
	relazione("B", "FB", "LB", 2)
	relazione("B", "FB", "CB", 6)
	relazione("B", "FB", "SB", 1)
	componente("FC", "C", "7120300", "finito", "manuale")
	componente("LC", "C", "7120301", "sciolto", "manuale")
	componente("CC", "C", "7120302", "commerciale", "manuale")
	relazione("C", "FC", "LC", 2)
	relazione("C", "FC", "CC", 8)
	deroga := func(nome, comp, tipo string) {
		t.Helper()
		var id uuid.UUID
		riga(&id, `INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, $3, 'prova ACME', $4) RETURNING deroga_id`,
			s.thread["C"], s.comp[comp], tipo, s.operatore)
		s.deroghe[nome] = id
	}
	deroga("cad", "FC", "cad_3d")
	deroga("2d", "LC", "disegno_2d")
	return s
}

// terna: un fabbisogno come lo confronta la L4 (componente, tipo di documento, bloccante).
type terna struct {
	comp      uuid.UUID
	tipo      string
	bloccante bool
}

func (x terna) String() string {
	return fmt.Sprintf("%s/%s/%v", x.comp.String()[24:], x.tipo, x.bloccante)
}

func ternePerTesto(m map[terna]bool) string {
	var out []string
	for k := range m {
		out = append(out, k.String())
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// TestL4LEquivalenzaDeiFabbisogniConLaVista (T-E1R-03; decisioni sull'analista, punti 1 e 2; PO-33 come L4 sintetica;
// A1c-L4, famiglia B5): l'equivalenza con le esclusioni nominate, la risoluzione delle regole di Go contro la vista, le
// note delle regole del cliente, le deroghe; la lettura non cambia il database e ValutaProdotti non ci parla.
func TestL4LEquivalenzaDeiFabbisogniConLaVista(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaEquivalenza(t, p)
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	m := motoreACME(t)

	prima := testutil.FotoDelDatabase(t, p)
	f, err := caricatore.Carica(context.Background(), pr, caricatore.Richiesta{Thread: []uuid.UUID{s.thread["A"], s.thread["B"], s.thread["C"]}})
	if err != nil {
		t.Fatalf("Carica: %v", err)
	}
	reg.Azzera()
	valutati := map[uuid.UUID]valutazione.ValutazioneProdotti{}
	threads := map[uuid.UUID]fotorfq.Thread{}
	for _, th := range f.Thread {
		v, err := valutazione.ValutaProdotti(f, th, m, nil)
		if err != nil {
			t.Fatalf("ValutaProdotti %s: %v", th.ID, err)
		}
		valutati[th.ID], threads[th.ID] = v, th
	}
	if len(reg.Voci()) != 0 {
		t.Errorf("ValutaProdotti ha parlato con il database: %v", reg.Voci())
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("la lettura ha cambiato il database: %v", d)
	}
	if len(valutati) != 3 {
		t.Fatalf("thread letti %d", len(valutati))
	}

	// Gli insiemi, con le esclusioni nominate.
	a1c, vista := map[terna]bool{}, map[terna]bool{}
	e1, e2, e3 := map[terna]bool{}, map[terna]bool{}, map[terna]bool{}
	nelPerimetro := map[uuid.UUID]bool{}
	inA1c := map[uuid.UUID]bool{}
	for id, v := range valutati {
		th := threads[id]
		attivi := map[uuid.UUID]bool{}
		for _, k := range th.Componenti {
			if k.ArchiviatoIl == nil {
				attivi[k.ID] = true
			}
		}
		for _, pv := range v.ProdottiValutati {
			if pv.ComponenteID == nil {
				t.Fatalf("%s: il target senza componente", pv.Rif)
			}
			// il perimetro, calcolato qui dalle relazioni confermate fra componenti attivi (R62 e A)
			coda := []uuid.UUID{*pv.ComponenteID}
			nelPerimetro[*pv.ComponenteID] = true
			for len(coda) > 0 {
				x := coda[0]
				coda = coda[1:]
				for _, r := range th.Relazioni {
					if r.PadreID == x && attivi[r.FiglioID] && !nelPerimetro[r.FiglioID] {
						nelPerimetro[r.FiglioID] = true
						coda = append(coda, r.FiglioID)
					}
				}
			}
			for _, x := range pv.Documenti.Voci {
				k := terna{x.ComponenteID, x.TipoDocumento, true}
				inA1c[x.ComponenteID] = true
				switch {
				case x.NotaRegola == valutazione.NotaSchemaSenza2D || x.NotaRegola == valutazione.NotaRegolaClienteSenza2D:
					e1[k] = true
				case x.TipoComponente == "finito" && x.TipoDocumento == "disegno_2d" && x.NotaRegola == valutazione.NotaRegolaCliente2DNonBloccante:
					e2[terna{x.ComponenteID, x.TipoDocumento, false}] = true
				default:
					a1c[k] = true
				}
				if want := valutazione.CalcolataDaVista; x.TipoDocumento == "disegno_2d" {
					if x.CalcolataDa != valutazione.CalcolataDaGo {
						t.Errorf("%s: il 2D è calcolato in Go: %+v", k, x)
					}
				} else if x.CalcolataDa != want {
					t.Errorf("%s: dalla vista: %+v", k, x)
				}
			}
			for _, x := range pv.Documenti.NonBloccanti {
				a1c[terna{x.ComponenteID, x.TipoDocumento, false}] = true
				inA1c[x.ComponenteID] = true
			}
			if len(pv.Documenti.Previsti) != 0 {
				t.Errorf("%s: previsti %+v (E4: nessuna rimozione; nessuna struttura)", pv.Rif, pv.Documenti.Previsti)
			}
		}
		for _, r := range th.Fascicolo {
			k := terna{r.ComponenteID, r.TipoDocumento, r.Bloccante}
			switch {
			case !nelPerimetro[r.ComponenteID]:
				e3[k] = true
			case e2[k]:
			default:
				vista[k] = true
			}
		}
	}
	nome := map[uuid.UUID]string{}
	for k, id := range s.comp {
		nome[id] = k
	}
	nomi := func(m map[terna]bool) string {
		var out []string
		for k := range m {
			out = append(out, nome[k.comp]+"/"+k.tipo)
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	if got := nomi(e1); got != "CB/disegno_2d CC/disegno_2d FA/disegno_2d LB/disegno_2d" {
		t.Errorf("E1 (le voci che la vista non ha): %s", got)
	}
	if got := nomi(e2); got != "FB/disegno_2d" {
		t.Errorf("E2 (il 2D del finito con la riga non bloccante): %s", got)
	}
	if got := nomi(e3); got != "OA/cad_3d OA/disegno_2d" {
		t.Errorf("E3 (la vista fuori dal perimetro): %s", got)
	}
	for k := range a1c {
		if !vista[k] {
			t.Errorf("A1c ha %s/%s/%v, la vista no", nome[k.comp], k.tipo, k.bloccante)
		}
	}
	for k := range vista {
		if !a1c[k] {
			t.Errorf("la vista ha %s/%s/%v, A1c no", nome[k.comp], k.tipo, k.bloccante)
		}
	}
	if len(a1c) != 16 {
		t.Errorf("le terne confrontate %d: %s", len(a1c), ternePerTesto(a1c))
	}
	for id := range nelPerimetro {
		if !inA1c[id] {
			t.Errorf("il componente %s del perimetro non compare in A1c", nome[id])
		}
	}
	if nelPerimetro[s.comp["OA"]] || nelPerimetro[s.comp["RA"]] || inA1c[s.comp["OA"]] || inA1c[s.comp["RA"]] {
		t.Error("lo sciolto fuori dal perimetro e l'archiviato non sono nel perimetro né in A1c")
	}

	// La risoluzione delle regole di Go per i nodi proposti contro la vista, per ogni componente confermato attivo.
	for id, th := range threads {
		for _, k := range th.Componenti {
			if k.ArchiviatoIl != nil {
				for _, r := range th.Fascicolo {
					if r.ComponenteID == k.ID {
						t.Errorf("la vista ha una riga dell'archiviato %s", nome[k.ID])
					}
				}
				continue
			}
			regole, cliente := valutazione.FabbisogniDelTipo(k.Tipo, th.Fabbisogni)
			goSet, vistaSet := map[string]bool{}, map[string]bool{}
			for _, r := range regole {
				goSet[fmt.Sprintf("%s/%v", r.TipoDocumento, r.Bloccante)] = true
			}
			for _, r := range th.Fascicolo {
				if r.ComponenteID == k.ID {
					vistaSet[fmt.Sprintf("%s/%v", r.TipoDocumento, r.Bloccante)] = true
				}
			}
			if fmt.Sprint(goSet) != fmt.Sprint(vistaSet) {
				t.Errorf("%s (thread %s): le regole di Go %v, la vista %v", nome[k.ID], id, goSet, vistaSet)
			}
			attesoCliente := (k.Tipo == "finito" || k.Tipo == "sciolto" || k.Tipo == "commerciale") && th.ID != s.thread["C"] ||
				k.Tipo == "sottoassieme" && th.ID == s.thread["B"]
			if cliente != attesoCliente {
				t.Errorf("%s: regole del cliente %v", nome[k.ID], cliente)
			}
		}
	}

	// PO-33 come L4 sintetica: le note delle regole del cliente.
	voce := func(thread, comp, tipo string) valutazione.VoceFabbisogno {
		t.Helper()
		v := valutati[s.thread[thread]]
		for _, pv := range v.ProdottiValutati {
			for _, x := range pv.Documenti.Voci {
				if x.ComponenteID == s.comp[comp] && x.TipoDocumento == tipo {
					return x
				}
			}
		}
		t.Fatalf("nessuna voce %s/%s", comp, tipo)
		return valutazione.VoceFabbisogno{}
	}
	nonBloccante := func(thread, comp, tipo string) valutazione.FabbisognoInformativo {
		t.Helper()
		for _, pv := range valutati[s.thread[thread]].ProdottiValutati {
			if n, ok := nonBloccanteDi(pv.Documenti, s.comp[comp], tipo); ok {
				return n
			}
		}
		t.Fatalf("nessun non bloccante %s/%s", comp, tipo)
		return valutazione.FabbisognoInformativo{}
	}
	if x := voce("A", "FA", "disegno_2d"); !x.Bloccante || x.NotaRegola != valutazione.NotaRegolaClienteSenza2D || !x.RegolaCliente || !x.Invariante {
		t.Errorf("il finito senza 2D e senza cad_3d: %+v", x)
	}
	for _, pv := range valutati[s.thread["A"]].ProdottiValutati {
		if haVoce(pv.Documenti, s.comp["FA"], "cad_3d") {
			t.Error("il finito senza il cad_3d del cliente: lo STEP strutturale resta l'asse della fonte, non una voce")
		}
	}
	for _, c := range [][3]string{{"A", "LA", "disegno_2d"}, {"A", "CA", "disegno_2d"}, {"B", "SB", "disegno_2d"}} {
		if n := nonBloccante(c[0], c[1], c[2]); n.NotaRegola != valutazione.NotaRegolaCliente2DNonBloccante {
			t.Errorf("%s: la riga esplicita non bloccante sul componente (K-01 A): %+v", c[1], n)
		}
	}
	if x := voce("B", "FB", "disegno_2d"); !x.Bloccante || x.NotaRegola != valutazione.NotaRegolaCliente2DNonBloccante {
		t.Errorf("il finito con la riga esplicita non bloccante resta bloccante (R99 A): %+v", x)
	}
	for _, c := range [][2]string{{"LB", "sciolto"}, {"CB", "commerciale"}} {
		if x := voce("B", c[0], "disegno_2d"); !x.Bloccante || x.NotaRegola != valutazione.NotaRegolaClienteSenza2D || x.Categoria == "" {
			t.Errorf("%s, il %s senza 2D nelle regole del cliente: %+v", c[0], c[1], x)
		}
	}
	if x := voce("C", "CC", "disegno_2d"); x.NotaRegola != valutazione.NotaSchemaSenza2D || x.RegolaCliente || x.Categoria != valutazione.CategoriaCommerciale {
		t.Errorf("il commerciale con le regole di default: %+v", x)
	}

	// Le deroghe: sul cad_3d vale come nella vista (S3), sul 2D non lo sostituisce (R62 D.4), con la diagnostica.
	if x := voce("C", "FC", "cad_3d"); esitoDi(x) != "presente/" || x.DerogaID == nil || *x.DerogaID != s.deroghe["cad"] {
		t.Errorf("la deroga sul cad_3d: %+v", x)
	}
	if x := voce("C", "LC", "disegno_2d"); esitoDi(x) != "manca/derogato_non_sostituisce_2d" || x.DerogaID == nil || *x.DerogaID != s.deroghe["2d"] {
		t.Errorf("la deroga sul 2D: %+v", x)
	}
	if d := conCodice(valutati[s.thread["C"]].Diagnostiche, valutazione.CodiceDocumentiDerogaNonSostituisce2D); len(d) != 1 {
		t.Errorf("diagnostiche %+v", d)
	}
	for _, pv := range valutati[s.thread["C"]].ProdottiValutati {
		if statoDi(pv.Documenti) != "incompleta/voce_mancante aperto/fonte_non_confermata" {
			t.Errorf("la completezza della RFQ C: %s", statoDi(pv.Documenti))
		}
	}
}
