// Package cockpit incorpora nel binario migrazione, template e statici (embed.FS):
// per aggiornare la postazione si sostituisce un solo file.
package cockpit

import "embed"

//go:embed migrations/0001_schema.sql web/templates/*.html web/static/*
var FS embed.FS
