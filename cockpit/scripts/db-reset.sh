#!/usr/bin/env bash
# Ricrea lo schema di sviluppo da zero (regola: una sola migrazione fino alla produzione).
set -euo pipefail
PSQL="${PSQL:-/c/Program Files/PostgreSQL/18/bin/psql.exe}"
export PGPASSWORD="${PGPASSWORD:-cockpit_dev}"
DB="${COCKPIT_DB:-cockpit_dev}"
cd "$(dirname "$0")/.."
"$PSQL" -U cockpit -h localhost -d "$DB" -v ON_ERROR_STOP=1 -q -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
"$PSQL" -U cockpit -h localhost -d "$DB" -v ON_ERROR_STOP=1 -q -f migrations/0001_schema.sql
echo "schema ricreato su $DB"
