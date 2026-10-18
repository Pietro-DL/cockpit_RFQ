//go:build integrazione

package testutil

import "testing"

// TestSchemaVuotoPerLaCorsa e' il preambolo di ogni corsa L4 su cockpit_test_b (PM23 §13; giro 1, MIG-06): dopo la
// prenotazione nel semaforo e la verifica del DSN, lo schema torna vuoto con SchemaVuoto (che controlla il database
// collegato: «test» nel nome), e i pacchetti della corsa migrano da zero con il loro binario. Cosi' un binario della 22
// e uno della 23 che usano lo stesso database di prova non si fermano l'uno sullo schema dell'altro. Sta solo nella QA.
func TestSchemaVuotoPerLaCorsa(t *testing.T) {
	SchemaVuoto(t, Pool(t))
}
