#!/usr/bin/env bash
# Run the parser/config unit test suite.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

exec go test ./... "$@"
