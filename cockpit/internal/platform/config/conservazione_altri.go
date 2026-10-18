//go:build !windows

package config

import "path/filepath"

// unitaDiRete: fuori da Windows non ci sono lettere di unita'; una condivisione montata in una cartella non si
// riconosce da qui (limite dichiarato: il server di produzione e' Windows).
func unitaDiRete(string) bool { return false }

// percorsoFinale: fuori da Windows, i collegamenti seguiti da filepath.EvalSymlinks.
func percorsoFinale(p string) (string, error) { return filepath.EvalSymlinks(p) }
