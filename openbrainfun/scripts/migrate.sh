#!/usr/bin/env bash
set -euo pipefail
: "${OPENBRAIN_DATABASE_URL:=postgres://postgres:postgres@127.0.0.1:5432/openbrain?sslmode=disable}"
for f in migrations/*.sql; do
  echo "applying $f"
  psql "$OPENBRAIN_DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"
done
