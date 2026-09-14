// Package cockpit incorpora nel binario migrazioni, template e statici (embed.FS):
// per aggiornare la postazione si sostituisce un solo file.
package cockpit

import "embed"

//go:embed migrations/*.sql web/templates/*.html web/static/*
var FS embed.FS
