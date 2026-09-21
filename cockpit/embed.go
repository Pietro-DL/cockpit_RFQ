// Package cockpit incorpora nel binario migrazioni, template e statici (embed.FS):
// per aggiornare la postazione si sostituisce un solo file.
package cockpit

import "embed"

// I file dei worker Python viaggiano dentro il binario: la pagina Postazioni ne fa un pacchetto
// gia' configurato (D22), e la versione del worker non puo' allontanarsi da quella del server.
//
// Dal 7C.1 viaggia anche installa-postazione.ps1: e' cio' che rende il pacchetto installabile senza
// aprire PowerShell a mano (attivita' pianificate, certificato, dipendenze).
//
//go:embed migrations/*.sql web/templates/*.html web/static/* workers/*.py workers/requirements.txt workers/installa-postazione.ps1
var FS embed.FS
