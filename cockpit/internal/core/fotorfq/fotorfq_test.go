// L1 — i record della fotografia (A1b-02; piano A, 5.4.2, par.3.4.2): l'impronta dei fatti non dipende dalla
// forma del JSON con cui sono arrivati, i numeri restano testo, e un payload che non ha una forma unica è un
// errore, mai un'impronta inventata. Più la versione dello schema, fissata qui (5.6.1).
//
// I fatti di questi test sono inventati (ACME, codici di fantasia): il repository è pubblico, e un'impronta
// non ha bisogno di dati veri per dimostrare che dà sempre gli stessi byte.
package fotorfq

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// shaDi: lo sha256 in esadecimale, calcolato qui senza jsoncanonico, per fissare il valore a mano.
func shaDi(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// TestLImprontaDeiFattiNonDipendeDallaForma (A1b-02): chiavi permutate, spazi e a capo, «\u00e8» contro «è»
// danno la stessa impronta, ed è lo sha256 del canonico scritto a mano; «800» e «800.0» danno impronte
// diverse, e la prova lo fissa.
func TestLImprontaDeiFattiNonDipendeDallaForma(t *testing.T) {
	const canonico = `{"esito":{"stato":"ok"},"nodi":[{"chiave":"#10","nome_grezzo":"ACME7000100 è"}],"qta":800}`
	forme := []string{
		canonico,
		`{"qta":800,"nodi":[{"nome_grezzo":"ACME7000100 è","chiave":"#10"}],"esito":{"stato":"ok"}}`,
		"{\r\n  \"esito\" : { \"stato\" : \"ok\" },\r\n  \"nodi\" : [ { \"chiave\" : \"#10\", \"nome_grezzo\" : \"ACME7000100 \\u00e8\" } ],\r\n  \"qta\" : 800\r\n}",
		`{"esito":{"stato":"ok"},"nodi":[{"chiave":"#10","nome_grezzo":"ACME7000100 \u00e8"}],"qta":800}`,
	}
	atteso := shaDi(canonico)
	for i, f := range forme {
		got, err := ImprontaPayload(json.RawMessage(f))
		if err != nil {
			t.Fatalf("forma %d: %v", i, err)
		}
		if got != atteso {
			t.Errorf("forma %d: impronta %s, attesa %s (lo sha256 del canonico scritto a mano)", i, got, atteso)
		}
	}

	// I numeri sono testo: 800 e 800.0 sono due letterali diversi, quindi due impronte. Se un giorno il
	// canonico normalizzasse i numeri, questa prova va riscritta con «Riscritta per …».
	intero, err := ImprontaPayload(json.RawMessage(`{"qta":800}`))
	if err != nil {
		t.Fatal(err)
	}
	decimale, err := ImprontaPayload(json.RawMessage(`{"qta":800.0}`))
	if err != nil {
		t.Fatal(err)
	}
	if intero == decimale {
		t.Error("800 e 800.0 danno la stessa impronta: i numeri non sono più copiati come testo")
	}
	if decimale != shaDi(`{"qta":800.0}`) {
		t.Errorf("il decimale non è stato copiato com'era: %s", decimale)
	}
}

// TestUnPayloadSenzaFormaUnicaNonHaImpronta (A1b-02): vuoto, non JSON, chiave ripetuta, UTF-8 non valido,
// surrogato solo, testo dopo il valore → errore, mai un'impronta.
func TestUnPayloadSenzaFormaUnicaNonHaImpronta(t *testing.T) {
	casi := map[string]json.RawMessage{
		"nil":                nil,
		"vuoto":              json.RawMessage(""),
		"non JSON":           json.RawMessage(`{"qta":`),
		"chiave ripetuta":    json.RawMessage(`{"qta":1,"qta":2}`),
		"UTF-8 non valido":   json.RawMessage("{\"nome\":\"ACME\xff\"}"),
		"surrogato solo":     json.RawMessage(`{"nome":"\ud800"}`),
		"testo dopo il JSON": json.RawMessage(`{"qta":1} {"qta":2}`),
	}
	for nome, p := range casi {
		t.Run(nome, func(t *testing.T) {
			if h, err := ImprontaPayload(p); err == nil {
				t.Errorf("impronta %s per un payload senza forma unica", h)
			}
		})
	}
}

// TestLaVersioneDelloSchemaDellaFotografiaEUno (5.6.1): la versione la fissa la prova del pacchetto che la
// possiede. Se cambia, questa prova si riscrive con il titolo «Riscritta per …».
func TestLaVersioneDelloSchemaDellaFotografiaEUno(t *testing.T) {
	if VersioneSchema != 1 {
		t.Fatalf("fotorfq.VersioneSchema = %d, attesa 1", VersioneSchema)
	}
}
