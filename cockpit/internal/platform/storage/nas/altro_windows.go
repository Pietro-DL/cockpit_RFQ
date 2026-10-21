//go:build windows

package nas

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// I codici di Windows che servono qui (winerror.h).
const (
	erroreFileNonTrovato     syscall.Errno = 2
	errorePercorsoNonTrovato syscall.Errno = 3
	erroreAccessoNegato      syscall.Errno = 5
	erroreCondivisione       syscall.Errno = 32 // ERROR_SHARING_VIOLATION
	erroreBlocco             syscall.Errno = 33 // ERROR_LOCK_VIOLATION
	erroreFileEsiste         syscall.Errno = 80 // ERROR_FILE_EXISTS
	erroreGiaEsiste          syscall.Errno = 183
	erroreDiscoPieno         syscall.Errno = 112 // ERROR_DISK_FULL
	erroreSpazioUtente       syscall.Errno = 39  // ERROR_HANDLE_DISK_FULL
	erroreNonSupportato      syscall.Errno = 50  // ERROR_NOT_SUPPORTED
	erroreFunzioneNonValida  syscall.Errno = 1   // ERROR_INVALID_FUNCTION
)

// rinominaSenzaSostituire: MoveFileW senza MOVEFILE_REPLACE_EXISTING. Fallisce se la destinazione esiste
// (fs.ErrExist), non lascia mai una destinazione parziale (sullo stesso volume e' una rinomina), e un file aperto da un
// altro programma senza la condivisione in cancellazione la fa fallire con una violazione di condivisione.
func rinominaSenzaSostituire(da, a string) error {
	pda, err := syscall.UTF16PtrFromString(da)
	if err != nil {
		return err
	}
	pa, err := syscall.UTF16PtrFromString(a)
	if err != nil {
		return err
	}
	if err := syscall.MoveFile(pda, pa); err != nil {
		var e syscall.Errno
		if errors.As(err, &e) {
			switch e {
			case erroreFileEsiste, erroreGiaEsiste:
				return &os.LinkError{Op: "rinomina", Old: da, New: a, Err: fs.ErrExist}
			case erroreFileNonTrovato, errorePercorsoNonTrovato:
				return &os.LinkError{Op: "rinomina", Old: da, New: a, Err: fs.ErrNotExist}
			case erroreNonSupportato, erroreFunzioneNonValida:
				return &os.LinkError{Op: "rinomina", Old: da, New: a, Err: ErrPromozioneNonDisponibile}
			}
		}
		return &os.LinkError{Op: "rinomina", Old: da, New: a, Err: err}
	}
	return nil
}

// nascondi marca il nome di servizio come nascosto (FILE_ATTRIBUTE_HIDDEN): il punto iniziale su Windows non nasconde
// niente. Un errore non ferma la rimozione: il nome resta privato lo stesso.
func nascondi(p string) {
	pp, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return
	}
	attr, err := syscall.GetFileAttributes(pp)
	if err != nil {
		return
	}
	_ = syscall.SetFileAttributes(pp, attr|syscall.FILE_ATTRIBUTE_HIDDEN)
}

// apriEsclusivo apre un file in lettura senza nessuna condivisione: se un altro programma lo tiene aperto, anche con
// la condivisione in cancellazione, l'apertura fallisce con una violazione di condivisione.
func apriEsclusivo(p string) (*os.File, error) {
	pp, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(pp, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "apri esclusivo", Path: p, Err: err}
	}
	return os.NewFile(uintptr(h), p), nil
}

// fileInUso: l'errore di un file aperto da un altro programma (violazione di condivisione o di blocco). Un accesso
// negato su una rinomina di un file aperto senza la condivisione in cancellazione vale lo stesso.
func fileInUso(err error) bool {
	var e syscall.Errno
	if errors.As(err, &e) {
		return e == erroreCondivisione || e == erroreBlocco || e == erroreAccessoNegato
	}
	return false
}

// spazioEsaurito: disco pieno o quota esaurita.
func spazioEsaurito(err error) bool {
	var e syscall.Errno
	if errors.As(err, &e) {
		return e == erroreDiscoPieno || e == erroreSpazioUtente
	}
	return false
}
