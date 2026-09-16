// Package cockpit incorpora nel binario migrazioni, template e statici (embed.FS):
// per aggiornare la postazione si sostituisce un solo file.
package cockpit

import "embed"

// I file dei worker Python viaggiano dentro il binario: la pagina Postazioni ne fa un pacchetto
// gia' configurato (D22), e la versione del worker non puo' allontanarsi da quella del server.
//
//go:embed migrations/*.sql web/templates/*.html web/static/* workers/*.py workers/requirements.txt
var FS embed.FS
