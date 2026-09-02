#!/usr/bin/env bash
# Build and run SimpleCiscoTUI.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Optional: source a local .env (CISCO_HOST, CISCO_USERNAME, …) if present.
if [[ -f "$ROOT/.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "$ROOT/.env"
    set +a
fi

exec go run . "$@"
