//go:build integrazione

// L4 — «Conferma l'albero» contro PostgreSQL vero (giro 4, fase 4.4a.1b; domande 27 = A, 28 = A, 29b = A, 30 seconda
// risposta, 6a, 6b; studio docs/specs/studio_albero_distinta_29-09.md § 2.8 e § 2.9): la conferma scrive in una
// transazione i pezzi, i tipi, le revisioni, i legami e le righe degli STEP con il segno «confermato nell'albero»; si
// rifiuta senza scrivere niente con una firma vecchia, una proposta commerciale aperta, la BOM congelata, e annulla
// tutto se un legame chiude un ciclo a meta'; il commerciale nasce solo con il ✓ di una persona; la cascata toglie un
// assieme e lascia il figlio in comune; il segno lo leggono F7 (niente «da rivedere»), F8 (i file si preselezionano) e
// il gate. Dalla verifica della fase: un aggancio per codice di prima si decide solo perche' il riepilogo lo elenca;
// «Riapri il nodo» riporta nell'albero un nodo che la conferma ha tolto; il riepilogo dice l'effetto di un cambio di
// tipo su un componente che c'e'. Solo dati inventati (ACME, 712…).

package fascicolo_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// bozzaSu e' la bozza disegnata sull'albero di adesso: il formato e la base (la firma dell'albero).
func (b *banco) bozzaSu(bozza fascicolo.BozzaAlbero) fascicolo.BozzaAlbero {
	b.t.Helper()
	var a fascicolo.AlberoProposto
	ok(b.t, b.tx(func(q *db.Queries) (err error) {
		a, err = fascicolo.LeggiAlberoProposto(b.ctx, q, b.thread, analizzatoreProva)
		return err
	}))
	bozza.Formato, bozza.Base = fascicolo.FormatoBozza, a.Firma
	return bozza
}

// riepilogoAlbero e' il riepilogo che la persona legge prima di confermare.
func (b *banco) riepilogoAlbero(bozza fascicolo.BozzaAlbero) fascicolo.RiepilogoAlbero {
	b.t.Helper()
	var r fascicolo.RiepilogoAlbero
	ok(b.t, b.tx(func(q *db.Queries) (err error) {
		_, r, err = fascicolo.LeggiRiepilogo(b.ctx, q, b.thread, analizzatoreProva, &bozza)
		return err
	}))
	return r
}

// confermaAlbero e' la POST della conferma: la bozza e la firma che la persona rimanda, in una transazione.
func (b *banco) confermaAlbero(bozza fascicolo.BozzaAlbero, firma string) (string, error) {
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ConfermaAlbero(b.ctx, q, b.thread, b.utente, analizzatoreProva, bozza, firma)
	})
}

// confermaVista: la bozza sull'albero di adesso, il riepilogo, la conferma con la sua firma. Deve riuscire.
func (b *banco) confermaVista(bozza fascicolo.BozzaAlbero) (string, fascicolo.RiepilogoAlbero) {
	b.t.Helper()
	bozza = b.bozzaSu(bozza)
	r := b.riepilogoAlbero(bozza)
	if !r.Confermabile {
		b.t.Fatalf("il riepilogo non e' confermabile: %v", r.Blocchi)
	}
	msg, err := b.confermaAlbero(bozza, r.Firma)
	if err != nil {
		b.t.Fatalf("la conferma: %v", err)
	}
	return msg, r
}

// laWorking: i componenti (codice:tipo:origine:rev, con «archiviato») e gli archi (padre>figlio*qta:origine) della RFQ.
func (b *banco) laWorking() string {
	return uno[string](b, `SELECT concat_ws(' # ',
		(SELECT string_agg(codice || ':' || tipo || ':' || origine || ':' || coalesce(rev, '-') || CASE WHEN archiviato_il IS NULL THEN '' ELSE ':archiviato' END,
		        ' ' ORDER BY codice) FROM componente WHERE thread_id = $1),
		(SELECT string_agg(p.codice || '>' || f.codice || '*' || r.qta || ':' || r.origine, ' ' ORDER BY p.codice, f.codice)
		   FROM componente_relazione r JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id
		  WHERE r.thread_id = $1))`, b.thread)
}

// righeAlbero: le righe degli STEP della RFQ, «file:chiave:stato» con «+segno» se hanno il segno dell'albero e «da»
// se le ha decise una persona.
func (b *banco) righeDegliStep() string {
	return uno[string](b, `SELECT concat_ws(' # ',
		(SELECT string_agg(a.nome_file || ':' || cp.chiave || ':' || cp.stato || CASE WHEN cp.evidenza ? 'albero' THEN '+segno' ELSE '' END ||
		        CASE WHEN cp.deciso_da IS NOT NULL THEN '+da' ELSE '' END, ' ' ORDER BY a.nome_file, cp.chiave)
		   FROM componente_proposta cp JOIN allegato a ON a.allegato_id = cp.allegato_id WHERE cp.thread_id = $1),
		(SELECT string_agg(a.nome_file || ':' || r.padre_chiave || '>' || r.figlio_chiave || ':' || r.stato ||
		        CASE WHEN r.evidenza ? 'albero' THEN '+segno' ELSE '' END || CASE WHEN r.deciso_da IS NOT NULL THEN '+da' ELSE '' END,
		        ' ' ORDER BY a.nome_file, r.padre_chiave, r.figlio_chiave)
		   FROM relazione_proposta r JOIN allegato a ON a.allegato_id = r.allegato_id WHERE r.thread_id = $1))`, b.thread)
}

// Prova (studio § 2.8; 27 = A, 29b = A): la conferma dell'albero della scena scrive tutto in una transazione. Nascono
// 7120010 e 7120011 (assiemi) e 7121003 e 7121005 (particolari), con l'origine dello STEP e chi ha confermato; i legami
// con le quantita' dei file (7121003 sta sotto 7120010 ×2 e sotto 7120011 ×1: un componente solo); 7121098 e 7121099,
// tolti nella bozza, restano senza padri: 7121099 ha un disegno e si archivia, 7121098 non ha storia e si elimina. Le
// righe degli STEP (tranne la radice del prodotto, che non e' un pezzo da decidere) sono decise da una persona con il
// segno. Dopo, l'albero e' tutto «nella distinta», e la bozza vuota non ha niente da fare.
func TestLaConfermaDellAlberoScriveInUnaTransazione(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	msg, r := b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}}})
	if len(r.Nuovi) != 4 || len(r.LegamiNuovi) != 5 || len(r.Fuori) != 2 {
		t.Errorf("il riepilogo visto: nuovi %d, legami %d, fuori %+v", len(r.Nuovi), len(r.LegamiNuovi), r.Fuori)
	}
	for _, c := range []string{"4 pezzi nuovi", "5 legami nuovi", "2 legami tolti", "Archiviati, senza padri: 7121099", "Eliminati, senza padri e senza storia: 7121098"} {
		if !strings.Contains(msg, c) {
			t.Errorf("la frase: manca %q in %q", c, msg)
		}
	}
	atteso := "7120001:finito:manuale:- 7120010:sottoassieme:step:- 7120011:sottoassieme:step:- 7121003:sciolto:step:- 7121005:sciolto:step:- " +
		"7121099:sciolto:manuale:-:archiviato # 7120001>7120010*1:step 7120001>7120011*2:step 7120010>7121003*2:step 7120010>7121005*1:step 7120011>7121003*1:step"
	if got := b.laWorking(); got != atteso {
		t.Errorf("la working:\n%s\natteso:\n%s", got, atteso)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND confermato_da = $2 AND codice IN ('7120010', '7120011', '7121003', '7121005')`,
		b.thread, b.utente); n != 4 {
		t.Errorf("i pezzi nuovi confermati da chi conferma: %d", n)
	}
	atteso = "7120001.stp:#1:aperta 7120001.stp:#2:confermata+segno+da 7120001.stp:#3:confermata+segno+da 7120001.stp:#4:confermata+segno+da " +
		"7120010.stp:#1:duplicato+segno+da 7120010.stp:#2:duplicato+segno+da 7120010.stp:#3:confermata+segno+da # " +
		"7120001.stp:#1>#2:confermata+segno+da 7120001.stp:#1>#3:confermata+segno+da 7120001.stp:#3>#4:confermata+segno+da " +
		"7120010.stp:#1>#2:confermata+segno+da 7120010.stp:#1>#3:confermata+segno+da"
	if got := b.righeDegliStep(); got != atteso {
		t.Errorf("le righe degli STEP:\n%s\natteso:\n%s", got, atteso)
	}
	// il segno di un arco dice il legame, con chi e la firma del riepilogo
	var segno fascicolo.SegnoAlbero
	raw := uno[[]byte](b, `SELECT r.evidenza -> 'albero' FROM relazione_proposta r JOIN allegato a ON a.allegato_id = r.allegato_id
		WHERE a.nome_file = '7120010.stp' AND r.padre_chiave = '#1' AND r.figlio_chiave = '#2'`)
	ok(t, json.Unmarshal(raw, &segno))
	if segno.Da != b.utente || segno.Firma != r.Firma || segno.Padre.UUID != b.codiceComp("7120010") || segno.Figlio.UUID != b.codiceComp("7121003") {
		t.Errorf("il segno dell'arco 7120010 → 7121003: %+v", segno)
	}
	// dopo: tutto nella distinta, e la bozza vuota non ha niente da fare
	dopo := b.riepilogoAlbero(b.bozzaSu(fascicolo.BozzaAlbero{}))
	if !dopo.Confermabile || len(dopo.Nuovi) != 0 || len(dopo.LegamiNuovi) != 0 || len(dopo.Ritrovati) != 0 || len(dopo.LegamiTolti) != 0 {
		t.Errorf("dopo la conferma la bozza vuota: %+v", dopo)
	}
}

// Prova (studio § 2.8): la conferma si rifiuta senza scrivere niente — ogni tabella dello schema ha le stesse righe con
// lo stesso contenuto prima e dopo — con la firma vuota o sbagliata, con la firma di un riepilogo che nel frattempo e'
// cambiato (un collega ha scartato un nodo), con la BOM congelata (D26), e annulla tutto quando un legame chiude un
// ciclo con un arco della working fuori dall'albero, dopo aver gia' fatto nascere dei pezzi.
func TestLaConfermaSiRifiutaSenzaScrivereNiente(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	bozza := b.bozzaSu(fascicolo.BozzaAlbero{})
	r := b.riepilogoAlbero(bozza)
	prima := b.fotoTabelle()
	for _, c := range []struct{ nome, firma, frase string }{
		{"firma vuota", "", "rileggi il riepilogo"},
		{"firma sbagliata", "abc", "rileggi il riepilogo"},
	} {
		_, err := b.confermaAlbero(bozza, c.firma)
		deveRifiutare(t, err, c.frase)
		if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
			t.Fatalf("%s: la conferma rifiutata ha scritto in %s", c.nome, d)
		}
	}

	// un collega scarta 7121005 dopo che la persona ha letto il riepilogo: la sua firma e' vecchia
	pid := uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND codice = '7121005'`, b.thread)
	_, err := b.gesto(func(q *db.Queries) (string, error) { return fascicolo.ScartaNodo(b.ctx, q, b.thread, pid, b.utente) })
	ok(t, err)
	prima = b.fotoTabelle()
	_, err = b.confermaAlbero(bozza, r.Firma)
	deveRifiutare(t, err, "è cambiato da quando hai letto il riepilogo")
	if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
		t.Fatalf("firma vecchia: la conferma rifiutata ha scritto in %s", d)
	}

	// la BOM congelata (D26): la conferma non comincia
	fase := uno[uuid.UUID](b, `SELECT fase_log_id FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, b.thread)
	v1 := uno[uuid.UUID](b, `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		VALUES ($1, 1, 'preventivo', 'FATTIBILITA', $2, 'prova', $3) RETURNING bom_versione_id`, b.thread, fase, b.utente)
	b.esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() WHERE bom_versione_id = $1`, v1, b.utente)
	bozza = b.bozzaSu(fascicolo.BozzaAlbero{})
	r = b.riepilogoAlbero(bozza)
	prima = b.fotoTabelle()
	_, err = b.confermaAlbero(bozza, r.Firma)
	deveRifiutare(t, err, "la BOM è congelata nella V1: si conferma l'albero solo aprendo una revisione")
	if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
		t.Fatalf("D26: la conferma rifiutata ha scritto in %s", d)
	}
}

// Prova (studio § 2.8, «il ventesimo arco che chiude un ciclo annulla tutto»): 7120010 e 7121005 ci sono gia', fuori
// dall'albero del prodotto, con l'arco 7121005 → 7120010. L'albero (che vede solo quello che si raggiunge dal prodotto)
// propone 7120010 → 7121005, e il riepilogo e' confermabile; la conferma fa nascere 7120011 e 7121003, e al legame
// 7120010 → 7121005 collega trova il ciclo con la working intera: rifiuto, e niente resta scritto.
func TestLaConfermaAnnullaTuttoSeUnLegameChiudeUnCiclo(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	a10 := b.componente("7120010", db.TipoComponenteSottoassieme)
	a05 := b.componente("7121005", db.TipoComponenteSottoassieme)
	b.arco(a05, a10, 1)
	bozza := b.bozzaSu(fascicolo.BozzaAlbero{})
	r := b.riepilogoAlbero(bozza)
	if !r.Confermabile || len(r.Ritrovati) != 2 {
		t.Fatalf("il riepilogo: confermabile %v, ritrovati %+v, blocchi %v", r.Confermabile, r.Ritrovati, r.Blocchi)
	}
	prima := b.fotoTabelle()
	_, err := b.confermaAlbero(bozza, r.Firma)
	deveRifiutare(t, err, "chiuderebbe un ciclo")
	if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
		t.Fatalf("il ciclo a meta': sono rimaste scritture in %s", d)
	}
}

// scenaVite e' la RFQ del prodotto 7120001 (la famiglia ACME dei codici 712…) con lo STEP del prodotto: 7120010 e,
// sotto il prodotto, la vite normata 7121007 «VITE TCEI ISO 4762 M6X16», che il riconoscitore propone particolare
// commerciale.
func (b *banco) scenaVite() {
	b.t.Helper()
	b.conRegole(`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`)
	b.prodottoConfermato("7120001")
	b.stepLetto("7120001.stp", strings.Repeat("b1", 32), fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010", "#3=7121007_VITE TCEI ISO 4762 M6X16"},
		archi: []string{"#1>#2", "#1>#3*4"}})
}

// Prova (domanda 30, seconda risposta; 6a): il tipo commerciale lo scrive la conferma solo con il ✓ di una persona, e
// registra chi. Senza risposta la conferma si rifiuta (la proposta e' aperta) e non scrive niente; con il ✗ la vite
// nasce particolare, e il segno ricorda il ✗ con il motivo (i «mancati» del censimento); con il ✓ nasce particolare
// commerciale, confermato da chi conferma, e il segno ricorda il ✓; scegliere «particolare commerciale» con la tendina
// e' lo stesso gesto (il segno lo dice).
func TestLaConfermaScriveIlCommercialeSoloConIlSi(t *testing.T) {
	commerciale := func(b *banco) (tipo string, s fascicolo.SegnoCommerciale) {
		tipo = uno[string](b, `SELECT tipo::text || ':' || (confermato_da = $2)::text FROM componente WHERE thread_id = $1 AND codice = '7121007'`,
			b.thread, b.utente)
		raw := uno[[]byte](b, `SELECT evidenza -> 'albero' -> 'commerciale' FROM componente_proposta WHERE thread_id = $1 AND codice = '7121007'`, b.thread)
		ok(b.t, json.Unmarshal(raw, &s))
		return tipo, s
	}
	t.Run("senza risposta si rifiuta", func(t *testing.T) {
		b := nuovoBanco(t)
		b.scenaVite()
		bozza := b.bozzaSu(fascicolo.BozzaAlbero{})
		r := b.riepilogoAlbero(bozza)
		if r.Confermabile || len(r.Commerciali) != 1 || r.Commerciali[0].Stato != fascicolo.CommercialeAperta {
			t.Fatalf("la proposta aperta: %+v %v", r.Commerciali, r.Blocchi)
		}
		prima := b.fotoTabelle()
		_, err := b.confermaAlbero(bozza, r.Firma)
		deveRifiutare(t, err, "1 proposta commerciale senza ✓ o ✗")
		if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
			t.Fatalf("la conferma rifiutata ha scritto in %s", d)
		}
	})
	t.Run("il ✗", func(t *testing.T) {
		b := nuovoBanco(t)
		b.scenaVite()
		b.confermaVista(fascicolo.BozzaAlbero{Commerciali: []fascicolo.RispostaBozza{{Nodo: "cod:7121007", Risposta: fascicolo.RispostaNo}}})
		if tipo, s := commerciale(b); tipo != "sciolto:true" || s.Risposta != fascicolo.RispostaNo || !strings.Contains(s.Motivo, "ISO 4762") {
			t.Errorf("il ✗: %s, segno %+v", tipo, s)
		}
	})
	t.Run("il ✓", func(t *testing.T) {
		b := nuovoBanco(t)
		b.scenaVite()
		msg, _ := b.confermaVista(fascicolo.BozzaAlbero{Commerciali: []fascicolo.RispostaBozza{{Nodo: "cod:7121007", Risposta: fascicolo.RispostaSi}}})
		if tipo, s := commerciale(b); tipo != "commerciale:true" || s.Risposta != fascicolo.RispostaSi || s.Tendina {
			t.Errorf("il ✓: %s, segno %+v", tipo, s)
		}
		if da := uno[uuid.UUID](b, `SELECT (evidenza -> 'albero' ->> 'da')::uuid FROM componente_proposta WHERE thread_id = $1 AND codice = '7121007'`,
			b.thread); da != b.utente || !strings.Contains(msg, "1 particolare commerciale confermato") {
			t.Errorf("chi ha confermato: %s; %q", da, msg)
		}
	})
	t.Run("la tendina e' il ✓", func(t *testing.T) {
		b := nuovoBanco(t)
		b.scenaVite()
		b.confermaVista(fascicolo.BozzaAlbero{Tipi: []fascicolo.TipoBozza{{Nodo: "cod:7121007", Tipo: db.TipoComponenteCommerciale}}})
		if tipo, s := commerciale(b); tipo != "commerciale:true" || s.Risposta != fascicolo.RispostaSi || !s.Tendina {
			t.Errorf("la tendina: %s, segno %+v", tipo, s)
		}
	})
}

// Prova (29b = A, con i figli in comune): confermato l'albero, 7120011 ha sotto 7121003 (in comune con 7120010) e un
// pezzo messo a mano, 7129800. Una seconda conferma toglie 7120011: vanno via i suoi legami; 7121003 resta sotto
// 7120010, il suo altro padre; 7120011 (con le sue righe decise: ha storia) si archivia, 7129800 (senza storia) si
// elimina. Le righe di 7120011, decise con la prima conferma, sono decise di nuovo: scartate, «tolto nell'albero
// confermato», e l'albero non lo ripropone.
func TestLaConfermaTogliUnAssiemeELasciaIlFiglioInComune(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	_, primo := b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}}})
	mano := b.componente("7129800", db.TipoComponenteSciolto)
	b.arco(b.codiceComp("7120011"), mano, 3)

	msg, r := b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7120011"}}})
	if len(r.Cascata) != 1 || strings.Join(r.Cascata[0].Vanno, " ") != "7120011 7129800" || len(r.Cascata[0].Restano) != 1 ||
		r.Cascata[0].Restano[0].Codice != "7121003" || strings.Join(r.Cascata[0].Restano[0].Padri, " ") != "7120010" {
		t.Errorf("la cascata del riepilogo: %+v", r.Cascata)
	}
	if !strings.Contains(msg, "Archiviati, senza padri: 7120011") || !strings.Contains(msg, "Eliminati, senza padri e senza storia: 7129800") {
		t.Errorf("la frase: %q", msg)
	}
	atteso := "7120001:finito:manuale:- 7120010:sottoassieme:step:- 7120011:sottoassieme:step:-:archiviato 7121003:sciolto:step:- " +
		"7121005:sciolto:step:- 7121099:sciolto:manuale:-:archiviato # 7120001>7120010*1:step 7120010>7121003*2:step 7120010>7121005*1:step"
	if got := b.laWorking(); got != atteso {
		t.Errorf("la working:\n%s\natteso:\n%s", got, atteso)
	}
	if got := uno[string](b, `SELECT string_agg(r.padre_chiave || '>' || r.figlio_chiave || ':' || r.stato || ':' || coalesce(r.nota, ''), ' ' ORDER BY r.padre_chiave, r.figlio_chiave)
		FROM relazione_proposta r JOIN allegato a ON a.allegato_id = r.allegato_id WHERE a.nome_file = '7120001.stp'`); got !=
		"#1>#2:confermata: #1>#3:scartata:tolto nell'albero confermato #3>#4:scartata:tolto nell'albero confermato" {
		t.Errorf("gli archi del file del prodotto: %s", got)
	}
	if got := uno[string](b, `SELECT stato || ':' || coalesce(nota, '') || ':' || coalesce(jsonb_array_length(evidenza -> 'storia'), 0) FROM componente_proposta
		WHERE thread_id = $1 AND codice = '7120011'`, b.thread); got != "scartata:tolto nell'albero confermato:1" {
		t.Errorf("la riga di 7120011: %s", got)
	}
	// la decisione di prima va nella storia con il suo segno (dalla verifica della fase: prima il segno si perdeva)
	if got := uno[string](b, `SELECT (evidenza -> 'storia' -> -1 ->> 'evento') || ':' || (evidenza -> 'storia' -> -1 ->> 'stato') || ':' ||
		(evidenza -> 'storia' -> -1 -> 'albero' ->> 'firma') FROM componente_proposta WHERE thread_id = $1 AND codice = '7120011'`, b.thread); got !=
		"deciso_nell_albero:confermata:"+primo.Firma {
		t.Errorf("la storia della riga di 7120011: %s (la prima firma %s)", got, primo.Firma)
	}
	a := b.bozzaSu(fascicolo.BozzaAlbero{})
	dopo := b.riepilogoAlbero(a)
	if !dopo.Confermabile || len(dopo.Nuovi) != 0 || len(dopo.Ritrovati) != 0 || len(dopo.LegamiNuovi) != 0 {
		t.Errorf("dopo, l'albero ripropone qualcosa: %+v", dopo)
	}
}

// Prova (risposta 28: una revisione aggiorna lo stesso pezzo, non ne crea uno nuovo): 7121003 c'e' nella distinta con
// la rev A, sotto 7120010; la riga ancora aperta dello STEP di 7120010 dice rev B. Il riepilogo lo dice («A → B»), e la
// conferma cambia la revisione dello stesso componente: nessun 7121003 nuovo.
func TestLaConfermaAggiornaLaRevisione(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	a10 := b.componente("7120010", db.TipoComponenteSottoassieme)
	p03 := b.componente("7121003", db.TipoComponenteSciolto)
	b.esegui(`UPDATE componente SET rev = 'A' WHERE componente_id = $1`, p03)
	b.arco(b.codiceComp("7120001"), a10, 1)
	b.arco(a10, p03, 2)
	b.esegui(`UPDATE componente_proposta cp SET rev = 'B' FROM allegato a WHERE a.allegato_id = cp.allegato_id AND a.nome_file = '7120010.stp'
		AND cp.codice = '7121003'`)
	msg, r := b.confermaVista(fascicolo.BozzaAlbero{})
	if len(r.Revisioni) != 1 || r.Revisioni[0].Codice != "7121003" || r.Revisioni[0].Da != "A" || r.Revisioni[0].A != "B" || r.Revisioni[0].Componente != p03 {
		t.Errorf("le revisioni del riepilogo: %+v", r.Revisioni)
	}
	if got := uno[string](b, `SELECT string_agg(componente_id::text || ':' || coalesce(rev, '-'), ' ') FROM componente WHERE thread_id = $1 AND codice = '7121003'`,
		b.thread); got != p03.String()+":B" || !strings.Contains(msg, "1 revisione nuova") {
		t.Errorf("7121003 dopo la conferma: %s; %q", got, msg)
	}
}

// Prova (studio § 2.9, F7): dopo la conferma nessun pezzo e nessun legame dell'albero e' «da rivedere», anche sotto il
// primo livello e senza nessuno STEP autorizzato: la Provenienza legge il segno. La controprova: tolto il segno dalle
// righe (la stessa decisione senza «confermato nell'albero»), i pezzi nati dallo STEP e i loro legami tornano «accettato
// da uno STEP non autorizzato».
func TestDopoLaConfermaF7NonSegnaDaRivedere(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}}})
	forme := func() fascicolo.FormeLegacy {
		var d fascicolo.DatiRiapertura
		ok(t, b.tx(func(q *db.Queries) (err error) { d, err = fascicolo.LeggiRiapertura(b.ctx, q, b.thread); return err }))
		return fascicolo.ClassificaFormeLegacy(d)
	}
	f := forme()
	if len(f.ComponentiDaRivedere) != 0 || len(f.ArchiDaRivedere) != 0 {
		t.Errorf("dopo la conferma: da rivedere %+v, archi %+v", f.ComponentiDaRivedere, f.ArchiDaRivedere)
	}
	b.esegui(`UPDATE componente_proposta SET evidenza = evidenza - 'albero' WHERE thread_id = $1`, b.thread)
	b.esegui(`UPDATE relazione_proposta SET evidenza = evidenza - 'albero' WHERE thread_id = $1`, b.thread)
	f = forme()
	var comp []string
	for _, x := range f.ComponentiDaRivedere {
		comp = append(comp, x.Componente+":"+strings.Join(x.Etichette, ","))
	}
	if strings.Join(comp, " ") != "7120010:accettato da uno STEP non autorizzato 7120011:accettato da uno STEP non autorizzato "+
		"7121003:accettato da uno STEP non autorizzato 7121005:accettato da uno STEP non autorizzato" || len(f.ArchiDaRivedere) != 5 {
		t.Errorf("la controprova, senza il segno: %v, archi %+v", comp, f.ArchiDaRivedere)
	}
}

// Prova (studio § 2.9, F8): il disegno 7121005.pdf e' di un pezzo del secondo livello (nello STEP di 7120010). Prima
// della conferma non si preseleziona (nessuna persona ha deciso il pezzo); dopo la conferma il flusso lo preseleziona
// sul pezzo, come dopo un'accettazione nell'autorita' di uno STEP autorizzato. La controprova: tolto il segno, non si
// preseleziona piu'.
func TestDopoLaConfermaF8PreselezionaIFileDeiPezzi(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	d05 := b.disegnoPdf(b.mail(), "7121005.pdf")
	b.rismista()
	if d := b.dest(d05); d.Preselezionabile {
		t.Fatalf("prima della conferma il disegno si preseleziona: %s", righeDest(d))
	}
	b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}}})
	b.rismista()
	d := b.dest(d05)
	if !d.Preselezionabile || len(d.Candidati) == 0 || d.Candidati[0].Codice != "7121005" {
		t.Errorf("dopo la conferma: %s, preselezionabile %v, motivo %q", righeDest(d), d.Preselezionabile, d.Motivo)
	}
	b.esegui(`UPDATE componente_proposta SET evidenza = evidenza - 'albero' WHERE thread_id = $1`, b.thread)
	b.esegui(`UPDATE relazione_proposta SET evidenza = evidenza - 'albero' WHERE thread_id = $1`, b.thread)
	b.rismista()
	if d := b.dest(d05); d.Preselezionabile {
		t.Errorf("la controprova, senza il segno: si preseleziona ancora (%s)", righeDest(d))
	}
}

// Prova (studio § 2.9, il gate): con lo STEP del prodotto autorizzato, i suoi figli diretti aperti fermano il
// congelamento («decisioni strutturali aperte negli STEP autorizzati»). Accettato a mano 7120011, il suo figlio nello
// STEP del prodotto (7121003) e' guida, e il gate lo dice fra gli avvisi. La conferma dell'albero decide tutto, con gli
// archi: il gate non si ferma piu' sulla struttura, e la guida decisa non e' piu' un avviso.
func TestIlGateDopoLaConferma(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, _, _ := b.scenaAlberoDB()
	a := uno[uuid.UUID](b, `SELECT allegato_id FROM allegato WHERE nome_file = '7120001.stp'`)
	testutil.AutorizzaStep(t, b.p, b.thread, prodotto, a, b.utente)
	strutturale := func(g fascicolo.Gate) string {
		for _, p := range g.Problemi {
			if strings.Contains(p, "decisioni strutturali aperte") {
				return p
			}
		}
		return ""
	}
	if p := strutturale(b.gateOra()); !strings.Contains(p, "4 decisioni strutturali aperte negli STEP autorizzati (2 figli diretti, 2 relazioni, 0 rimozioni)") {
		t.Fatalf("prima della conferma il gate: %q (%v)", p, b.gateOra().Problemi)
	}
	pid := uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#3'`, b.thread, a)
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, pid, b.utente, "")
	})
	ok(t, err)
	if g := b.gateOra(); strutturale(g) == "" || !strings.Contains(avvisiDi(g), "7120011") {
		t.Fatalf("con 7120011 accettato a mano: problemi %v, avvisi %s", g.Problemi, avvisiDi(g))
	}
	b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}}})
	g := b.gateOra()
	if p := strutturale(g); p != "" {
		t.Errorf("dopo la conferma il gate si ferma ancora sulla struttura: %q", p)
	}
	if s := b.strutturali(); s.NFigliDaDecidere+s.NArchiDaDecidere+s.NRimozioni != 0 {
		t.Errorf("GateStrutturale dopo la conferma: %+v", s)
	}
	if strings.Contains(avvisiDi(g), "guida") {
		t.Errorf("dopo la conferma il gate avvisa della guida: %s", avvisiDi(g))
	}
}

// Prova (P13, U5; fase 4.4a.1b, dalla verifica): un aggancio per codice di prima dello Smistamento — la riga di
// 7121005 nello STEP di 7120010, duplicato con il componente e senza chi l'ha deciso — non e' una decisione. Il
// riepilogo lo elenca fra i ritrovati, con il perche' («agganciato per codice prima dello Smistamento»), e la conferma
// lo decide: diventa la decisione di chi conferma, con il segno, e com'era va nella storia. La seconda meta': una
// riga chiusa da un automatismo (scartata senza chi l'ha decisa) di un pezzo che il riepilogo non elenca — 7120010,
// gia' nella distinta — resta com'e' anche dopo una seconda conferma (prima la conferma decideva ogni riga senza chi
// l'aveva decisa, elencata o no).
func TestLaConfermaDecideSoloLAggancioPerCodiceCheIlRiepilogoElenca(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	c05 := b.componente("7121005", db.TipoComponenteSciolto)
	b.esegui(`UPDATE componente_proposta cp SET stato = 'duplicato', componente_id = $2, deciso_il = now() FROM allegato a
		WHERE a.allegato_id = cp.allegato_id AND a.nome_file = '7120010.stp' AND cp.codice = '7121005' AND cp.thread_id = $1`, b.thread, c05)
	msg, r := b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}}})
	var nuovi []string
	for _, n := range r.Nuovi {
		nuovi = append(nuovi, n.Codice)
	}
	if len(r.Ritrovati) != 1 || r.Ritrovati[0].Codice != "7121005" || r.Ritrovati[0].Componente != c05 ||
		!strings.Contains(r.Ritrovati[0].Perche, "agganciato per codice prima dello Smistamento") || strings.Join(nuovi, " ") != "7120010 7120011 7121003" {
		t.Errorf("il riepilogo: ritrovati %+v, nuovi %v", r.Ritrovati, nuovi)
	}
	if !strings.Contains(msg, "3 pezzi nuovi, 1 pezzo che c'era già") {
		t.Errorf("la frase: %q", msg)
	}
	riga := func(file, chiave string) string {
		return uno[string](b, `SELECT cp.stato || ':' || coalesce((cp.componente_id = $3)::text, '-') || ':' || coalesce((cp.deciso_da = $4)::text, '-') || ':' ||
			(cp.evidenza ? 'albero')::text || ':' || coalesce(cp.evidenza -> 'storia' -> -1 ->> 'evento', '-') || ':' ||
			coalesce(cp.evidenza -> 'storia' -> -1 ->> 'stato', '-')
			FROM componente_proposta cp JOIN allegato a ON a.allegato_id = cp.allegato_id WHERE a.nome_file = $1 AND cp.chiave = $2`,
			file, chiave, c05, b.utente)
	}
	if got := riga("7120010.stp", "#3"); got != "duplicato:true:true:true:deciso_nell_albero:duplicato" {
		t.Errorf("la riga agganciata dopo la conferma: %s", got)
	}
	if got := b.laWorking(); !strings.Contains(got, "7120010>7121005*1:step") || strings.Count(got, "7121005:") != 1 {
		t.Errorf("la working: %s", got)
	}

	// una riga chiusa da un automatismo su 7120010, che la seconda conferma non elenca: resta com'e'
	b.esegui(`UPDATE componente_proposta cp SET stato = 'scartata', componente_id = NULL, deciso_da = NULL, evidenza = cp.evidenza - 'albero'
		FROM allegato a WHERE a.allegato_id = cp.allegato_id AND a.nome_file = '7120001.stp' AND cp.chiave = '#2'`)
	_, r = b.confermaVista(fascicolo.BozzaAlbero{})
	if len(r.Ritrovati) != 0 || len(r.Nuovi) != 0 {
		t.Errorf("la seconda conferma: ritrovati %+v, nuovi %+v", r.Ritrovati, r.Nuovi)
	}
	if got := riga("7120001.stp", "#2"); got != "scartata:-:-:false:-:-" {
		t.Errorf("la riga chiusa da un automatismo, che il riepilogo non elenca, dopo la seconda conferma: %s", got)
	}
}

// Prova (studio § 2.5, «RiapriNodo li riporta»; fase 4.4a.1b, dalla verifica): la conferma toglie 7121005 (la sua riga
// e quella dell'arco 7120010 → 7121005 nello STEP di 7120010 diventano scartate, con il segno e la nota). «Riapri il
// nodo» sulla sua riga riapre anche l'arco che la stessa conferma aveva chiuso, con la storia: il nodo torna
// nell'albero proposto sotto 7120010, e la conferma dopo lo fa nascere. Senza l'arco il nodo non si raggiungerebbe piu'.
func TestRiapriIlNodoToltoNellaConfermaLoRiportaNellAlbero(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}, {Nodo: "cod:7121005"}}})
	if got := b.righeDegliStep(); !strings.Contains(got, "7120010.stp:#3:scartata+segno+da") || !strings.Contains(got, "7120010.stp:#1>#3:scartata+segno+da") {
		t.Fatalf("dopo la conferma che toglie 7121005: %s", got)
	}
	pid := uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND codice = '7121005'`, b.thread)
	msg, err := b.gesto(func(q *db.Queries) (string, error) { return fascicolo.RiapriNodo(b.ctx, q, b.thread, pid, b.utente) })
	ok(t, err)
	if msg != "7121005 riaperto: torna fra le proposte, con 1 legame dello STEP che la conferma dell'albero aveva tolto." {
		t.Errorf("la frase: %q", msg)
	}
	if got := b.righeDegliStep(); !strings.Contains(got, "7120010.stp:#3:aperta ") || !strings.Contains(got, "7120010.stp:#1>#3:aperta") ||
		strings.Contains(got, "7120010.stp:#1>#3:aperta+") {
		t.Errorf("le righe dopo la riapertura: %s", got)
	}
	if got := uno[string](b, `SELECT coalesce(cp.nota, '-') || ':' || (cp.evidenza -> 'storia' -> -1 ->> 'evento') || ':' || (cp.evidenza -> 'storia' -> -1 ->> 'nota') || ' ' ||
		coalesce(r.nota, '-') || ':' || (r.evidenza -> 'storia' -> -1 ->> 'evento') || ':' || (r.evidenza -> 'storia' -> -1 -> 'albero' ? 'firma')::text
		FROM componente_proposta cp JOIN relazione_proposta r ON r.allegato_id = cp.allegato_id AND r.figlio_chiave = cp.chiave WHERE cp.proposta_id = $1`, pid); got !=
		"-:nodo_riaperto:tolto nell'albero confermato -:arco_riaperto:true" {
		t.Errorf("la storia del nodo e dell'arco riaperti: %s", got)
	}
	var a fascicolo.AlberoProposto
	ok(t, b.tx(func(q *db.Queries) (err error) {
		a, err = fascicolo.LeggiAlberoProposto(b.ctx, q, b.thread, analizzatoreProva)
		return err
	}))
	if n, trovato := a.Nodo("cod:7121005"); !trovato || n.Stato != fascicolo.StatoAlberoProposto || strings.Join(n.Padri, " ") != "cod:7120010" {
		t.Fatalf("dopo la riapertura l'albero: %+v (trovato %v)", n, trovato)
	}
	msg, r := b.confermaVista(fascicolo.BozzaAlbero{})
	if len(r.Nuovi) != 1 || r.Nuovi[0].Codice != "7121005" || len(r.LegamiNuovi) != 1 || !strings.Contains(msg, "1 pezzo nuovo") {
		t.Errorf("la conferma dopo la riapertura: nuovi %+v, legami %+v, %q", r.Nuovi, r.LegamiNuovi, msg)
	}
	if got := b.laWorking(); !strings.Contains(got, "7120010>7121005*1:step") {
		t.Errorf("la working: %s", got)
	}
}

// Prova (fase 4.4a.1b, dalla verifica): per un componente che c'e' il riepilogo dice l'effetto del cambio di tipo,
// con le regole e le parole dell'anteprima della scheda, calcolato su come la conferma lo trovera'. 7120010, con il suo
// STEP autorizzato, diventa particolare commerciale e perde nella bozza i due figli: il riepilogo dice che
// l'autorizzazione si sospende (i figli che ha adesso nella working non lo spengono: la conferma li toglie prima), e la
// conferma la sospende. Con la BOM congelata il cambio di tipo e' spento, e lo Spento e' un blocco del riepilogo.
func TestIlRiepilogoDiceLEffettoDelCambioDiTipo(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaAlberoDB()
	b.confermaVista(fascicolo.BozzaAlbero{Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}}})
	c10 := b.codiceComp("7120010")
	testutil.AutorizzaStep(t, b.p, b.thread, c10, uno[uuid.UUID](b, `SELECT allegato_id FROM allegato WHERE nome_file = '7120010.stp'`), b.utente)
	if d := b.dichiarazioneDi(c10); d != "valida" {
		t.Fatalf("l'autorizzazione di 7120010: %s", d)
	}
	tipoDi := func(r fascicolo.RiepilogoAlbero, codice string) fascicolo.TipoRiepilogo {
		for _, x := range r.Tipi {
			if x.Codice == codice {
				return x
			}
		}
		return fascicolo.TipoRiepilogo{}
	}
	msg, r := b.confermaVista(fascicolo.BozzaAlbero{Tipi: []fascicolo.TipoBozza{{Nodo: "cod:7120010", Tipo: db.TipoComponenteCommerciale}},
		Tolti: []fascicolo.ToltoBozza{{Padre: "cod:7120010", Nodo: "cod:7121003"}, {Padre: "cod:7120010", Nodo: "cod:7121005"}}})
	x := tipoDi(r, "7120010")
	if x.A != db.TipoComponenteCommerciale || x.Spento != "" || len(x.Effetti) == 0 ||
		!strings.HasPrefix(x.Effetti[0], "si sospende l'autorizzazione di 7120010.stp per 7120010") {
		t.Errorf("l'effetto nel riepilogo: %+v", x)
	}
	if !strings.Contains(msg, "Sospesa l'autorizzazione di 7120010.stp per 7120010") || b.dichiarazioneDi(c10) == "valida" {
		t.Errorf("la conferma: %q, l'autorizzazione %s", msg, b.dichiarazioneDi(c10))
	}

	// la BOM congelata: il ritorno a sottoassieme e' spento, e il riepilogo lo dice come un blocco
	fase := uno[uuid.UUID](b, `SELECT fase_log_id FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, b.thread)
	v1 := uno[uuid.UUID](b, `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		VALUES ($1, 1, 'preventivo', 'FATTIBILITA', $2, 'prova', $3) RETURNING bom_versione_id`, b.thread, fase, b.utente)
	b.esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() WHERE bom_versione_id = $1`, v1, b.utente)
	r = b.riepilogoAlbero(b.bozzaSu(fascicolo.BozzaAlbero{Tipi: []fascicolo.TipoBozza{{Nodo: "cod:7120010", Tipo: db.TipoComponenteSottoassieme}}}))
	const spento = "7120010: la BOM è congelata nella V1: il tipo si cambia aprendo una revisione"
	if x := tipoDi(r, "7120010"); r.Confermabile || !strings.Contains(strings.Join(r.Blocchi, "|"), spento) || x.Spento == "" || len(x.Effetti) != 0 {
		t.Errorf("con la BOM congelata: confermabile %v, blocchi %v, tipo %+v", r.Confermabile, r.Blocchi, x)
	}
}
