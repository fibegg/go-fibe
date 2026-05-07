#!/usr/bin/env bash
set -euo pipefail

for file in migrations/*.sql; do
  grep -q -- "-- +goose Up" "$file"
  grep -q -- "-- +goose Down" "$file"
done

