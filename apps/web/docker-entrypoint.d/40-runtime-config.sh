#!/bin/sh
set -eu

APP_BASE_PATH_VALUE="${APP_BASE_PATH:-/}"
ESCAPED_APP_BASE_PATH="$(printf '%s' "$APP_BASE_PATH_VALUE" | sed 's/\\/\\\\/g; s/"/\\"/g')"
API_BASE_URL_VALUE="${API_BASE_URL:-/api}"
ESCAPED_API_BASE_URL="$(printf '%s' "$API_BASE_URL_VALUE" | sed 's/\\/\\\\/g; s/"/\\"/g')"

cat > /usr/share/nginx/html/config.js <<EOF
window.RUNTIME_CONFIG = {
  APP_BASE_PATH: "$ESCAPED_APP_BASE_PATH",
  API_BASE_URL: "$ESCAPED_API_BASE_URL"
}
EOF
