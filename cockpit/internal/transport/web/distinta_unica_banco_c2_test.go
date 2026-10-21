package web

// L1 — il banco finto della Distinta unica dopo la direttiva 35, blocco C-2 (parte interfaccia): le preassociazioni del
// formato 3 per allegato_id. Il banco ricontrolla le regole di R2 che la corsia persistenza ha scritto nel server
// (distintabozza/preassociazioni3.go, formato3.go) perché le prove nel browser vedano i gesti che il server rifiuterebbe:
// la forma {allegato_id, chiave_bozza}, lo stesso allegato una volta sola, appartenenza, natura e attività delle PB nuove o
// spostate, lo svincolo {allegato_id, componente_id} che cita un'associazione registrata. E la Prova 2 della direttiva
// sul banco: un file senza proposta di documento si preassocia. Dati inventati.

import (
	"fmt"
	"testing"
)

func TestBancoDistintaUnicaPreassociazioniPerAllegato(t *testing.T) {
	b := nuovoBancoDU(t)
	// la forma step di R2 vuole lo STEP documento confermato del prodotto (FONTE_NON_CONFERMATA, C-3): qui confermato da prima
	b.fonte["confermata"] = true
	base := "/thread/" + bdThread
	s6 := b.bdChiedi(t, "GET", base+"/sorgente/step/"+bdA1+"/anteprima?prodotto="+bdP1, nil, 200)
	g := s6["gruppi"].([]any)[0].(m)
	b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": "00000000-0000-4000-8000-0000000e5001",
		"crea": m{"partenza": "step", "sorgente": m{"allegato_id": bdA1, "sha256": bdShaStep, "gruppo": g["gruppo"]}, "impronta_contenuto": g["impronta_contenuto"]}}, 200)
	n := 0
	// salva: un salvataggio R2 con lo snapshot di R1 cambiato da cambia; torna la risposta
	salva := func(cambia func(s m), atteso int) m {
		t.Helper()
		r1 := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1, nil, 200)
		snap := r1["snapshot"].(m) // bdChiedi decodifica ogni risposta da capo: è già una copia
		cambia(snap)
		n++
		return b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": r1["bozza_id"], "versione_attesa": r1["versione"], "prodotto_id": bdP1, "vocabolario": 2,
			"chiave_salvataggio": fmt.Sprintf("00000000-0000-4000-8000-0000000e5%03d", 100+n), "snapshot": snap,
			"azioni": []any{m{"azione": "nodo_modificato", "chiave_bozza": "k-7", "prima": m{}, "dopo": m{}, "origine": "utente"}}}, atteso)
	}
	pb := func(l ...m) func(s m) {
		return func(s m) {
			v := []any{}
			for _, x := range l {
				v = append(v, x)
			}
			s["documenti_preassociati"] = v
		}
	}
	rifiuto := func(nome string, r m, codice, motivo string) {
		t.Helper()
		if r["codice"] != codice || bdStr(r["motivo"]) != motivo {
			t.Errorf("%s: %v, atteso %s %s", nome, r, codice, motivo)
		}
	}
	// la forma: un proposta_id non è più la chiave della PB (formato 2), lo stesso allegato non compare due volte
	rifiuto("PB con proposta_id", salva(pb(m{"proposta_id": bdProposta(bdA2), "chiave_bozza": "k-7"}), 422), "CORPO_NON_VALIDO", "")
	rifiuto("PB con un campo in più", salva(pb(m{"allegato_id": bdA2, "proposta_id": bdProposta(bdA2), "chiave_bozza": "k-7"}), 422), "CORPO_NON_VALIDO", "")
	rifiuto("lo stesso allegato due volte", salva(pb(m{"allegato_id": bdA2, "chiave_bozza": "k-7"}, m{"allegato_id": bdA2, "chiave_bozza": "k-11"}), 422), "CORPO_NON_VALIDO", "")
	// appartenenza, natura, attività
	rifiuto("allegato di un'altra RFQ", salva(pb(m{"allegato_id": "00000000-0000-4000-8000-0000000a0fff", "chiave_bozza": "k-7"}), 422), "FUORI_RFQ", "")
	// D-2 (c) [U], direttiva 39 §5: solo lo STEP legato come sorgente del finito (qui la sorgente della bozza) non si preassocia
	rifiuto("STEP sorgente", salva(pb(m{"allegato_id": bdA1, "chiave_bozza": "k-7"}), 422), "CORPO_NON_VALIDO", "legato_come_sorgente")
	// la provenienza la decide il server (direttiva 39 §4): la pagina non la mette su una PB nuova (salvo il registro di un
	// suggerimento accettato)
	rifiuto("PB nuova con la provenienza", salva(pb(m{"allegato_id": bdA2, "chiave_bozza": "k-7", "provenienza": "motore"}), 422), "CORPO_NON_VALIDO", "provenienza_dal_server")
	rifiuto("contenitore", salva(pb(m{"allegato_id": bdA7, "chiave_bozza": "k-7"}), 422), "CORPO_NON_VALIDO", "natura_non_preassociabile")
	b.mu.Lock()
	b.fileDi(bdA4).Stato = "altri_file"
	b.mu.Unlock()
	rifiuto("file in Altri file", salva(pb(m{"allegato_id": bdA4, "chiave_bozza": "k-7"}), 422), "FILE_NON_ATTIVO", "")
	b.mu.Lock()
	b.fileDi(bdA4).Stato = "attivo"
	b.mu.Unlock()
	if v := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1, nil, 200)["versione"]; bdNum(v) != 1 {
		t.Errorf("un salvataggio rifiutato ha scritto la bozza: versione %v", v)
	}
	// una voce estratta da un archivio si preassocia; la PB salvata e poi non attiva passa se non si tocca
	salva(pb(m{"allegato_id": bdA8, "chiave_bozza": "k-7"}), 200)
	b.mu.Lock()
	b.fileDi(bdA8).Stato = "altri_file"
	b.mu.Unlock()
	salva(func(s m) { s["nodi"].([]any)[0].(m)["descrizione"] = "radice, descritta" }, 200)
	rifiuto("PB non attiva spostata", salva(pb(m{"allegato_id": bdA8, "chiave_bozza": "k-11"}), 422), "FILE_NON_ATTIVO", "")
	b.mu.Lock()
	b.fileDi(bdA8).Stato = "attivo"
	b.mu.Unlock()
	// Prova 2 sul banco: un file senza proposta di documento (S1 con proposta_id e proposta null) si preassocia
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "senza-proposta", "attivo": true}, 204)
	s1 := b.bdChiedi(t, "GET", base+"/sorgente", nil, 200)
	for _, x := range s1["file"].([]any) {
		if f := x.(m); f["allegato_id"] == bdA6 && (f["proposta_id"] != nil || f["proposta"] != nil) {
			t.Errorf("S1 con l'interruttore senza-proposta dà ancora la proposta: %v", f)
		}
	}
	salva(pb(m{"allegato_id": bdA8, "chiave_bozza": "k-7"}, m{"allegato_id": bdA6, "chiave_bozza": "k-11"}), 200)
	r1 := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1, nil, 200)
	if got := r1["snapshot"].(m)["documenti_preassociati"].([]any); len(got) != 2 || got[0].(m)["allegato_id"] != bdA6 {
		t.Errorf("le PB salvate non sono per allegato_id, nell'ordine canonico: %v", got)
	}
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "senza-proposta", "attivo": false}, 204)
	// la provenienza decisa dal server: una PB nuova è manuale, rimandata come letta passa, cambiata dalla pagina no
	for _, x := range r1["snapshot"].(m)["documenti_preassociati"].([]any) {
		if d := x.(m); d["provenienza"] != "manuale" {
			t.Errorf("la PB nuova non è manuale: %v", d)
		}
	}
	salva(func(s m) { s["nodi"].([]any)[0].(m)["descrizione"] = "radice, descritta di nuovo" }, 200)
	rifiuto("provenienza cambiata dalla pagina", salva(func(s m) { s["documenti_preassociati"].([]any)[0].(m)["provenienza"] = "motore" }, 422), "CORPO_NON_VALIDO", "provenienza_dal_server")
	// uno STEP che non è la sorgente di un finito si preassocia come un PDF (D-2 (c))
	salva(func(s m) {
		s["documenti_preassociati"] = append(s["documenti_preassociati"].([]any), m{"allegato_id": bdA10, "chiave_bozza": "k-41"})
	}, 200)
	// lo svincolo: {allegato_id, componente_id}, con un'associazione registrata della coppia (nel banco il componente
	// del file); proposta_id è la forma di prima
	dec := func(d m) func(s m) {
		return func(s m) { s["decisioni"] = append(bdElenco(s["decisioni"]), d) }
	}
	rifiuto("svincolo con proposta_id", salva(dec(m{"id": "sv-1", "tipo": "scollega_documento", "proposta_id": bdProposta(bdA11), "componente_id": bdC40}), 422), "CORPO_NON_VALIDO", "")
	rifiuto("svincolo senza AR", salva(dec(m{"id": "sv-1", "tipo": "scollega_documento", "allegato_id": bdA2, "componente_id": bdC40}), 422), "CORPO_NON_VALIDO", "svincolo_senza_associazione")
	salva(dec(m{"id": "sv-1", "tipo": "scollega_documento", "allegato_id": bdA11, "componente_id": bdC40}), 200)
	// la conversione dice le PB tradotte e segnalate (qui nessuna: la bozza formato 2 del banco non ne ha)
	r1 = b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1, nil, 200)
	b.bdChiedi(t, "POST", base+"/motore-a/bozza/"+bdStr(r1["bozza_id"])+"/abbandona", m{"versione_attesa": r1["versione"]}, 200)
	b.bdChiedi(t, "POST", "/__banco/bozza-formato2", nil, 204)
	r1 = b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1, nil, 200)
	cv := b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": "00000000-0000-4000-8000-0000000e5901",
		"converti_da": m{"bozza_id": r1["bozza_id"], "versione_attesa": r1["versione"], "motivo": "prova"}}, 200)
	if bdNum(cv["preassociazioni_tradotte"]) != 0 || cv["preassociazioni_segnalate"] == nil {
		t.Errorf("la conversione senza la traccia delle preassociazioni: %v", cv)
	}
}
