//go:build windows

package config

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// procGetDriveTypeW: GetDriveTypeW di kernel32, per riconoscere una lettera mappata su una condivisione di rete
// (DRIVE_REMOTE). La libreria standard non la espone, e golang.org/x/sys non e' fra i moduli del progetto.
var procGetDriveTypeW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW")

// procGetFinalPathNameByHandleW: GetFinalPathNameByHandleW di kernel32, il percorso finale di un file aperto (dopo
// giunzioni, collegamenti simbolici e unita' SUBST, con il volume vero). Per la stessa ragione, da kernel32.
var procGetFinalPathNameByHandleW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")

// driveRemote: il valore di GetDriveTypeW per un'unita' di rete.
const driveRemote = 4

// unitaDiRete dice se la lettera del percorso («Z:») e' un'unita' di rete. Un percorso senza lettera non lo e' (un
// UNC lo riconosce percorsoUNC).
func unitaDiRete(p string) bool {
	vol := filepath.VolumeName(p)
	if len(vol) != 2 || vol[1] != ':' {
		return false
	}
	radice, err := syscall.UTF16PtrFromString(vol + `\`)
	if err != nil {
		return false
	}
	r, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(radice)))
	return r == driveRemote
}

// percorsoFinale risolve un percorso che esiste attraverso il suo handle (GetFinalPathNameByHandleW, VOLUME_NAME_DOS):
// le giunzioni (mount point), i collegamenti simbolici e le unita' SUBST si seguono fino al file vero, sul suo volume.
// Il prefisso «\\?\» si toglie («\\?\UNC\server\…» torna «\\server\…», e lo riconosce percorsoUNC). Se il percorso non
// si apre (permessi), si ripiega su filepath.EvalSymlinks: il controllo di prima, mai meno.
func percorsoFinale(p string) (string, error) {
	u, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return "", err
	}
	// accesso 0 e FILE_FLAG_BACKUP_SEMANTICS: basta per una cartella, senza leggerne il contenuto
	h, err := syscall.CreateFile(u, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return filepath.EvalSymlinks(p)
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 512)
	for {
		n, _, _ := procGetFinalPathNameByHandleW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
		if n == 0 {
			return filepath.EvalSymlinks(p) // l'API non risponde: il controllo di prima
		}
		if int(n) < len(buf) {
			return senzaPrefissoEsteso(syscall.UTF16ToString(buf[:n])), nil
		}
		buf = make([]uint16, n+1)
	}
}

// senzaPrefissoEsteso: «\\?\C:\x» → «C:\x»; «\\?\UNC\server\cond\x» → «\\server\cond\x».
func senzaPrefissoEsteso(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + strings.TrimPrefix(p, `\\?\UNC\`)
	case strings.HasPrefix(p, `\\?\`):
		return strings.TrimPrefix(p, `\\?\`)
	}
	return p
}
