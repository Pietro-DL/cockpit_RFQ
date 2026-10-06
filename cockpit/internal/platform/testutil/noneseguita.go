package testutil

import "testing"

// NonEseguita ferma la prova con «NON ESEGUITA: <motivo>» (t.Fatalf): manca un dato privato, la copia del dump o
// una risorsa obbligatoria, non c'è un difetto del codice (piano A, 3.3.9 e 6.4.7; R44). go test non dice mai «ok»
// per una prova reale che non ha girato, e non la salta mai: il riepilogo del par.1.8 la riconosce dal prefisso e
// la conta a parte, mai passata e mai fallita. Il motivo dice che cosa manca e come darlo.
func NonEseguita(t testing.TB, motivo string) {
	t.Helper()
	t.Fatalf("NON ESEGUITA: %s", motivo)
}
