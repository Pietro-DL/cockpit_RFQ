package rete

import (
	"path/filepath"
	"strings"
	"testing"
)

// Il materiale TLS porta il certificato pubblico in PEM (per il pacchetto della postazione) e sa dire
// quali nomi copre (7C.1, P1). La chiave privata non sta in PEM.
func TestIlMaterialePortaIlCertificatoPubblicoESaCheNomiCopre(t *testing.T) {
	dir := t.TempDir()
	cert, key := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	m, err := Prepara(cert, key, []string{"localhost", "10.0.0.7", "cockpit.azienda.local"})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Generato {
		t.Fatal("atteso un certificato generato ora")
	}
	if !strings.Contains(string(m.PEM), "BEGIN CERTIFICATE") || strings.Contains(string(m.PEM), "PRIVATE KEY") {
		t.Fatalf("PEM inatteso:\n%s", m.PEM)
	}
	for _, nome := range []string{"localhost", "10.0.0.7", "COCKPIT.azienda.local"} {
		if !m.Copre(nome) {
			t.Errorf("il certificato non copre %q (nomi: %v)", nome, m.Nomi)
		}
	}
	if m.Copre("10.0.0.8") || m.Copre("altro.azienda.local") {
		t.Error("il certificato copre un nome che non ha")
	}
	if !m.Copre("") {
		t.Error("un host vuoto non e' una richiesta: Copre deve dire si'")
	}

	// ricaricato dal disco: stesso certificato, stesso PEM, stessi nomi
	r, err := Prepara(cert, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Generato || r.Impronta != m.Impronta || string(r.PEM) != string(m.PEM) {
		t.Errorf("ricaricato diverso: generato=%v impronta uguale=%v pem uguale=%v", r.Generato, r.Impronta == m.Impronta, string(r.PEM) == string(m.PEM))
	}
	if !r.Copre("cockpit.azienda.local") {
		t.Errorf("i nomi non sopravvivono alla rilettura: %v", r.Nomi)
	}
}
