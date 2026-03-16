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

APP_BASE_PATH_VALUE="$(normalize_base_path "${APP_BASE_PATH:-/}")"
DEFAULT_API_BASE_URL='/api'
if [ "$APP_BASE_PATH_VALUE" != "/" ]; then
  DEFAULT_API_BASE_URL="${APP_BASE_PATH_VALUE}/api"
fi
API_BASE_URL_VALUE="${API_BASE_URL:-$DEFAULT_API_BASE_URL}"
ESCAPED_APP_BASE_PATH="$(printf '%s' "$APP_BASE_PATH_VALUE" | sed 's/\\/\\\\/g; s/"/\\"/g')"
ESCAPED_API_BASE_URL="$(printf '%s' "$API_BASE_URL_VALUE" | sed 's/\\/\\\\/g; s/"/\\"/g')"

cat > /usr/share/nginx/html/config.js <<EOF
window.RUNTIME_CONFIG = {
  APP_BASE_PATH: "$ESCAPED_APP_BASE_PATH",
  API_BASE_URL: "$ESCAPED_API_BASE_URL"
}
EOF
