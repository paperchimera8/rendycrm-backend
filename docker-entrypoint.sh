#!/bin/sh
set -eu

normalize_base_path() {
  value="$(printf '%s' "${1:-}" | sed 's#//*#/#g')"
  value="${value%/}"
  if [ -z "$value" ] || [ "$value" = "." ] || [ "$value" = "/" ]; then
    printf '/'
    return
  fi
  case "$value" in
    /*) printf '%s' "$value" ;;
    *) printf '/%s' "$value" ;;
  esac
}

escape_js() {
  printf '%s' "${1:-}" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

if [ -n "${POSTGRES_SSLROOTCERT_URL:-}" ]; then
  cert_path="${POSTGRES_SSLROOTCERT:-/run/certs/postgres-root.crt}"
  mkdir -p "$(dirname "$cert_path")"
  curl -fsSL "$POSTGRES_SSLROOTCERT_URL" -o "$cert_path"
  chmod 0600 "$cert_path"
  export POSTGRES_SSLROOTCERT="$cert_path"
fi

if [ -d "/web" ]; then
  app_base_path="$(normalize_base_path "${APP_BASE_PATH:-/}")"
  api_base_url="${API_BASE_URL:-/api}"
  cat > /web/config.js <<EOF
window.RUNTIME_CONFIG = window.RUNTIME_CONFIG || {
  APP_BASE_PATH: "$(escape_js "$app_base_path")",
  API_BASE_URL: "$(escape_js "$api_base_url")"
}
EOF
fi

exec /api "$@"
