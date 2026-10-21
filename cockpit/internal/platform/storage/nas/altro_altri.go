//go:build !windows

package nas

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// rinominaSenzaSostituire fuori da Windows: il link esclusivo (fallisce se la destinazione esiste), poi la rimozione
// del nome vecchio. Dove il link non c'e' (certe condivisioni montate) non si ripiega su os.Rename, che sostituisce:
// ErrPromozioneNonDisponibile.
func rinominaSenzaSostituire(da, a string) error {
	if err := os.Link(da, a); err != nil {
		switch {
		case errors.Is(err, fs.ErrExist), errors.Is(err, fs.ErrNotExist):
			return err
		case errors.Is(err, syscall.EPERM), errors.Is(err, syscall.ENOTSUP), errors.Is(err, syscall.EXDEV):
			return &os.LinkError{Op: "rinomina", Old: da, New: a, Err: ErrPromozioneNonDisponibile}
		}
		return err
	}
	return os.Remove(da)
}

// nascondi: fuori da Windows il punto iniziale basta.
func nascondi(string) {}

// apriEsclusivo fuori da Windows: un'apertura normale (limite dichiarato: il server di produzione e' Windows, e qui
// non c'e' un modo portabile di negare la condivisione).
func apriEsclusivo(p string) (*os.File, error) { return os.Open(p) }

// fileInUso: fuori da Windows un file aperto non impedisce rinomina e rimozione.
func fileInUso(err error) bool {
	return errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.ETXTBSY)
}

// spazioEsaurito: disco pieno o quota esaurita.
func spazioEsaurito(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}
