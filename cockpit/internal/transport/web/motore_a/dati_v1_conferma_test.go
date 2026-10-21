package motore_a

// L1 — V1, ondata C (emendamento U2 §2.4, §5.3; corsia motore): la data e l'autore della fonte e conferma_corrente nel
// modello di /dati, sulla scena ACME inventata (indice_test.go) e con righe sintetiche degli eventi della fonte.
//   - datiDellaFonte: la conferma dall'ultimo conferma_documento del documento, non seguito da una sostituzione; prima
//     di V1 (nessun evento) dal documento della fotografia, con il nome letto; l'autorizzazione dall'ultima
//     autorizzazione non seguita da una revoca; senza evento «non_registrato», mai ricostruita ([UR] Q13).
//   - confermaCorrente: assente senza conferme e per una conferma formato 1 o 2; per una formato 3 la verifica di adesso
//     «non calcolata» con il motivo (mai «sì» finché prima-istanza-1 non c'è); senza registro il motivo del mancato
//     confronto (senza la fonte nel registro, senza il registro di adesso), mai un «nessuna differenza» falso.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// maPersonaJSON: {utente_id, nome} di U2 §2.4, per le forme JSON delle prove.
type maPersonaJSON struct {
	UtenteID string `json:"utente_id"`
	Nome     string `json:"nome"`
}

func TestDatiDellaFonteDateEAutori(t *testing.T) {
	doc, altro, prod := maUUID(501), maUUID(502), maUUID(10)
	u1, u2, u3 := maUUID(601), maUUID(602), maUUID(603)
	t0 := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	ev := func(n int64, azione string, d uuid.UUID, autore uuid.UUID, nome string) db.ListEventiFonteDelThreadRow {
		return db.ListEventiFonteDelThreadRow{EventoID: n, ProdottoID: prod, Azione: azione, DocumentoID: uuid.NullUUID{UUID: d, Valid: true},
			AutoreID: autore, AutoreNome: nome, Quando: t0.Add(time.Duration(n) * time.Hour)}
	}
	foto := &fotorfq.Thread{Documenti: []fotorfq.DocumentoConfermato{{ID: doc, ConfermatoDa: u3, ConfermatoIl: t0.Add(-24 * time.Hour)}}}
	nuova := func(autorizzata bool) *FonteDelProdotto {
		d := doc
		return &FonteDelProdotto{DocumentoID: &d, Confermata: true, Autorizzata: autorizzata}
	}

	// conferma e autorizzazione con gli eventi: l'ultima conferma del documento, l'ultima autorizzazione
	r := &Risultato{Thread: foto, V1: DatiV1{EventiFonte: []db.ListEventiFonteDelThreadRow{
		ev(1, azioneConfermaDocumento, doc, u1, "Anna"), ev(2, azioneAutorizzazione, doc, u2, "Bruno"),
		ev(3, azioneConfermaDocumento, altro, u2, "Bruno"), ev(4, azioneConfermaDocumento, doc, u2, "Bruno")}}}
	f := nuova(true)
	datiDellaFonte(r, prod, f)
	if f.ConfermataIl == nil || !f.ConfermataIl.Equal(t0.Add(4*time.Hour)) || f.ConfermataDa == nil || *f.ConfermataDa != (Persona{u2, "Bruno"}) {
		t.Errorf("la conferma dagli eventi: %v %+v", f.ConfermataIl, f.ConfermataDa)
	}
	if f.AutorizzataIl == nil || !f.AutorizzataIl.Equal(t0.Add(2*time.Hour)) || *f.AutorizzataDa != (Persona{u2, "Bruno"}) || f.AutorizzataMotivo != "" {
		t.Errorf("l'autorizzazione dagli eventi: %v %+v %q", f.AutorizzataIl, f.AutorizzataDa, f.AutorizzataMotivo)
	}

	// la revoca dopo l'autorizzazione (e una fonte che dice ancora autorizzata): nessuna data, non_registrato
	r.V1.EventiFonte = append(r.V1.EventiFonte, ev(5, azioneRevoca, doc, u1, "Anna"))
	f = nuova(true)
	datiDellaFonte(r, prod, f)
	if f.AutorizzataIl != nil || f.AutorizzataMotivo != AutorizzazioneNonRegistrata {
		t.Errorf("dopo la revoca: %v %q", f.AutorizzataIl, f.AutorizzataMotivo)
	}
	// non autorizzata: nessun campo dell'autorizzazione
	f = nuova(false)
	datiDellaFonte(r, prod, f)
	if f.AutorizzataIl != nil || f.AutorizzataDa != nil || f.AutorizzataMotivo != "" {
		t.Errorf("non autorizzata: %+v", f)
	}

	// la conferma seguita da una sostituzione non vale: la data torna dal documento
	r.V1.EventiFonte = append(r.V1.EventiFonte, ev(6, azioneSostituzione, altro, u1, "Anna"))
	r.V1.NomiUtenti = map[uuid.UUID]string{u3: "Carla"}
	f = nuova(false)
	datiDellaFonte(r, prod, f)
	if f.ConfermataIl == nil || !f.ConfermataIl.Equal(t0.Add(-24*time.Hour)) || *f.ConfermataDa != (Persona{u3, "Carla"}) {
		t.Errorf("dopo la sostituzione: %v %+v", f.ConfermataIl, f.ConfermataDa)
	}

	// prima di V1: nessun evento, la conferma dal documento e l'autorizzazione non registrata
	r.V1.EventiFonte = nil
	f = nuova(true)
	datiDellaFonte(r, prod, f)
	if f.ConfermataIl == nil || *f.ConfermataDa != (Persona{u3, "Carla"}) || f.AutorizzataMotivo != AutorizzazioneNonRegistrata || f.AutorizzataDa != nil {
		t.Errorf("prima di V1: %+v", f)
	}
	// gli eventi di un altro prodotto non contano; senza documento niente
	r.V1.EventiFonte = []db.ListEventiFonteDelThreadRow{ev(1, azioneAutorizzazione, doc, u1, "Anna")}
	r.V1.EventiFonte[0].ProdottoID = maUUID(11)
	f = nuova(true)
	datiDellaFonte(r, prod, f)
	if f.AutorizzataMotivo != AutorizzazioneNonRegistrata {
		t.Errorf("l'evento di un altro prodotto: %+v", f)
	}
	g := &FonteDelProdotto{Autorizzata: true}
	datiDellaFonte(r, prod, g)
	if g.ConfermataIl != nil || g.AutorizzataMotivo != "" {
		t.Errorf("senza documento: %+v", g)
	}
}

func TestConfermaCorrenteNelModello(t *testing.T) {
	pid := maUUID(10)
	u := Persona{UtenteID: maUUID(601), Nome: "Anna"}
	il := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	pagina := func(r *Risultato) *Pagina { return ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Scrive: true}) }

	if p := pagina(maRisultatoACME()); p.ConfermaCorrente != nil {
		t.Errorf("senza conferme: %+v", p.ConfermaCorrente)
	}
	r := maRisultatoACME()
	r.V1.Conferme = map[uuid.UUID]ConfermaUltima{pid: {ConfermaID: maUUID(700), ProdottoID: pid, Numero: 1, Formato: 2, ConfermataIl: il, ConfermataDa: u}}
	if p := pagina(r); p.ConfermaCorrente != nil {
		t.Errorf("una conferma formato 2: %+v", p.ConfermaCorrente)
	}

	// formato 3 senza registro: la verifica non calcolata con il motivo, il confronto non fatto con il suo motivo
	r.V1.Conferme[pid] = ConfermaUltima{ConfermaID: maUUID(701), ProdottoID: pid, Numero: 2, Formato: 3, ConfermataIl: il, ConfermataDa: u,
		Criterio: "step", Valida: true}
	p := pagina(r)
	cc := p.ConfermaCorrente
	if cc == nil || cc.ConfermaID != maUUID(701) || cc.Numero != 2 || !cc.Valida || cc.ConfermataDa != u || cc.PropostaAggiornata != nil ||
		cc.PropostaNonConfrontata != MotivoConfermaSenzaRegistro {
		t.Fatalf("formato 3 senza registro: %+v", cc)
	}
	if cc.VerificaAdesso.Stato != StatoVerificaNonCalcolata || cc.VerificaAdesso.Verificata != nil || cc.VerificaAdesso.Criterio != "step" ||
		len(cc.VerificaAdesso.Motivi) != 1 || cc.VerificaAdesso.Motivi[0] != MotivoPrimaIstanzaNonPresente || cc.VerificaAdesso.Variazioni == nil {
		t.Errorf("la verifica di adesso: %+v", cc.VerificaAdesso)
	}
	b, _ := json.Marshal(cc)
	var chiavi map[string]json.RawMessage
	_ = json.Unmarshal(b, &chiavi)
	if _, ok := chiavi["proposta_aggiornata"]; ok {
		t.Errorf("senza confronto il JSON porta proposta_aggiornata: %s", b)
	}

	// con il registro: senza la fonte nel registro il gruppo corrente non si trova; con la fonte sullo STEP del gruppo
	// bom ma il registro di adesso che non si importa (la scena L1 non ha il grafo dello STEP) il motivo lo dice. Mai un
	// «nessuna differenza» falso. Il confronto vero sta in TestLaPropostaAggiornataNelleSeiClassi (valutazione) e in L4.
	imp := maUUID(702)
	c := r.V1.Conferme[pid]
	c.ImportazioneID = &imp
	r.V1.Conferme[pid] = c
	r.V1.Registri = map[uuid.UUID]importazione.Contenuto{imp: {}}
	if cc := pagina(r).ConfermaCorrente; cc == nil || cc.PropostaAggiornata != nil || cc.PropostaNonConfrontata != MotivoGruppoCorrenteAssente {
		t.Errorf("il registro senza fonte: %+v", cc)
	}
	var reg importazione.Contenuto
	reg.Proposta.Fonte = importazione.FonteProposta{AllegatoID: maUUID(31).String(), Sha256: maSha(1)}
	reg.Gruppo = importazione.Gruppo{Chiave: propostabom.GruppoBOM, Radice: maNodo(1, "#1")}
	r.V1.Registri[imp] = reg
	if cc := pagina(r).ConfermaCorrente; cc == nil || cc.PropostaAggiornata != nil || cc.PropostaNonConfrontata != MotivoRegistroNonImportato {
		t.Errorf("il registro di adesso che non si importa: %+v", cc)
	}
}
