//go:build privato

package testutil

// Privato è vero nelle corse compilate con il tag privato (piano A, 3.3.9; R24 a): le prove sulla copia del dump
// e sul dataset privato esistono solo lì, e senza il tag non si compilano.
const Privato = true
