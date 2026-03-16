#!/bin/sh
set -eu

API_BASE_URL_VALUE="${API_BASE_URL:-/api}"
ESCAPED_API_BASE_URL="$(printf '%s' "$API_BASE_URL_VALUE" | sed 's/\\/\\\\/g; s/"/\\"/g')"

cat > /usr/share/nginx/html/config.js <<EOF
window.RUNTIME_CONFIG = {
  API_BASE_URL: "$ESCAPED_API_BASE_URL"
}
EOF
