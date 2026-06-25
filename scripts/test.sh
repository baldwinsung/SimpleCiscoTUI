#!/usr/bin/env bash
# Run the parser test suite in the local virtualenv.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV="$ROOT/.venv"

if [[ ! -d "$VENV" ]]; then
    python3 -m venv "$VENV"
    "$VENV/bin/pip" install --quiet --upgrade pip
fi

"$VENV/bin/pip" install --quiet -r "$ROOT/requirements.txt"
"$VENV/bin/pip" install --quiet pytest

exec "$VENV/bin/python" -m pytest "$ROOT/tests" "$@"
