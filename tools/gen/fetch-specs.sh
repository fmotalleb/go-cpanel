#!/usr/bin/env bash
# Download the official cPanel & WHM OpenAPI documents and documentation
# catalogs from cPanel's developer portal (https://api.docs.cpanel.net).
set -euo pipefail
cd "$(dirname "$0")"

BASE="https://api.docs.cpanel.net"

curl -fsSL --retry 3 -o cpanel.openapi.yaml "$BASE/_bundle/specifications/cpanel.openapi.yaml"
curl -fsSL --retry 3 -o whm.openapi.yaml    "$BASE/_bundle/specifications/whm.openapi.yaml"
curl -fsSL --retry 3 -o cpanel.openapi.md   "$BASE/specifications/cpanel.openapi.md"
curl -fsSL --retry 3 -o whm.openapi.md      "$BASE/specifications/whm.openapi.md"

echo "downloaded:"
ls -la cpanel.openapi.yaml whm.openapi.yaml cpanel.openapi.md whm.openapi.md
