#!/bin/sh
set -eu

if [ -n "${POSTGRES_SSLROOTCERT_URL:-}" ]; then
  cert_path="${POSTGRES_SSLROOTCERT:-/run/certs/postgres-root.crt}"
  mkdir -p "$(dirname "$cert_path")"
  curl -fsSL "$POSTGRES_SSLROOTCERT_URL" -o "$cert_path"
  chmod 0600 "$cert_path"
  export POSTGRES_SSLROOTCERT="$cert_path"
fi

exec /api "$@"
