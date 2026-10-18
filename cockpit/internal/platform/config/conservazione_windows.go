//go:build windows

package config

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

// procGetDriveTypeW: GetDriveTypeW di kernel32, per riconoscere una lettera mappata su una condivisione di rete
// (DRIVE_REMOTE). La libreria standard non la espone, e golang.org/x/sys non e' fra i moduli del progetto.
var procGetDriveTypeW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW")

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
