#!/usr/bin/env bash
# Run SimpleCiscoTUI from a local virtualenv, creating it on first run.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV="$ROOT/.venv"

if [[ ! -d "$VENV" ]]; then
    echo "Creating virtualenv at $VENV …"
    python3 -m venv "$VENV"
    "$VENV/bin/pip" install --quiet --upgrade pip
    "$VENV/bin/pip" install --quiet -r "$ROOT/requirements.txt"
fi

# Optional: source a local .env (CISCO_HOST, CISCO_USERNAME, …) if present.
if [[ -f "$ROOT/.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "$ROOT/.env"
    set +a
fi

exec "$VENV/bin/python" -m simpleciscotui "$@"
