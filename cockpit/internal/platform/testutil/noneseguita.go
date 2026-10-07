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

// parteNonEseguita segna NON ESEGUITA una parte della prova, con il motivo, senza fermarla (t.Errorf): il resto
// gira e dice il suo esito, ma la prova non è mai verde (R44). Il prefisso è quello di NonEseguita, così il
// riepilogo del par.1.8 la riconosce allo stesso modo. Oggi la usa solo PoolDump, per le impronte che il manifest
// non dichiara (R117 b).
func parteNonEseguita(t testing.TB, motivo string) {
	t.Helper()
	t.Errorf("NON ESEGUITA: %s", motivo)
}
