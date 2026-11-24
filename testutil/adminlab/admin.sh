#!/bin/sh
# One admin request over TLS 1.3; prints the one-line JSON response (empty if the server refused the
# connection). Run from the repo root.
#   testutil/adminlab/admin.sh <host:port> <cert-name|none> '<json request>'
# cert-name selects certs/<name>.crt/.key as the client identity; "none" presents no certificate.
# The request may be "-" to read it from stdin (e.g. piped from testutil/adminlab/sign).
addr="$1"; who="$2"; req="$3"
[ "$req" = "-" ] && req="$(cat)"
set -- -quiet -no_ign_eof -tls1_3 -connect "$addr" -CAfile certs/ca.crt
[ "$who" != "none" ] && set -- "$@" -cert "certs/$who.crt" -key "certs/$who.key"
{ printf '%s\n' "$req"; sleep 1; } | openssl s_client "$@" 2>/dev/null
echo
