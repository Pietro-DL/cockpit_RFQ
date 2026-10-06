//go:build !privato

package testutil

// Privato è falso nelle corse senza il tag privato (piano A, 3.3.9; R24 a): le prove sulla copia del dump e sul
// dataset privato non ci sono, e una corsa che le richiede le conta «NON ESEGUITA (tag assente)».
const Privato = false
