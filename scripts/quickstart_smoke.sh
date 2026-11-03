#!/usr/bin/env bash
# Proves the README's "try it" stack works end to end, from outside the containers:
# log in through Dex, write a document through the editor's server, publish it, and
# read it back over the delivery API. Run it against a stack started with
# compose.try.yml. CI does that from a clean clone (.github/workflows/quickstart.yml).
#
# Every step checks an exact status code. The refusals come first, so a stack that
# has quietly opened up fails here rather than looking like a successful login.
set -euo pipefail

editor="${EDITOR_URL:-http://localhost:8080}"
api="${API_URL:-http://localhost:${API_PORT:-3000}}"
tenant=00000000-0000-4000-8000-000000000001
delivery_token=try-local-only-delivery
slug="smoke-$(date +%s)"

jar="$(mktemp)"
out="$(mktemp)"
trap 'rm -f "$jar" "$out"' EXIT

fail() {
  echo "FAIL: $1" >&2
  sed 's/^/  body: /' "$out" | cut -c1-300 >&2 || true
  exit 1
}

# expect <status> <name> <curl args...>: the response body lands in $out.
expect() {
  local want="$1" name="$2" got
  shift 2
  got="$(curl -sS -m 30 -o "$out" -w '%{http_code}' "$@")" || fail "$name: curl failed"
  [ "$got" = "$want" ] || fail "$name: expected $want, got $got"
  echo "ok   $name ($got)"
}

wait_for() {
  local url="$1"
  for _ in $(seq 1 90); do
    if curl -sS -m 3 -o /dev/null -f "$url" 2>/dev/null; then return 0; fi
    sleep 2
  done
  fail "$url did not answer within 3 minutes"
}

wait_for "$api/readyz"
wait_for "$editor/healthz"

# ── Refused before login ─────────────────────────────────────────────────────
expect 401 "no session: who am I" "$editor/auth/me"
expect 401 "no session: editor API through the proxy" "$editor/proxy/api/v1/schema"
expect 401 "no token: editor API directly" "$api/api/v1/schema"
expect 401 "no token: delivery API" -H "X-Scope-OrgID: $tenant" "$api/delivery/v1/post"

# ── Log in through the provider ──────────────────────────────────────────────
# Follows login -> Dex -> mock connector -> /auth/callback -> the editor's start page.
expect 200 "login through Dex" -L -c "$jar" -b "$jar" "$editor/auth/login"
expect 200 "session: who am I" -b "$jar" "$editor/auth/me"
grep -q '"sub"' "$out" || fail "session: no user in /auth/me"

# ── The API accepted the provider's token, and read the role from it ─────────
expect 200 "provider's token: schema through the proxy" -b "$jar" "$editor/proxy/api/v1/schema"

# ── Write, publish, read back ────────────────────────────────────────────────
expect 403 "cross-origin write refused" -b "$jar" -X POST \
  -H "Origin: https://evil.example" -H 'Content-Type: application/json' -d '{}' \
  "$editor/proxy/api/v1/documents"

expect 201 "create a draft" -b "$jar" -X POST \
  -H "Origin: $editor" -H 'Content-Type: application/json' \
  -d "{\"type\":\"post\",\"slug\":\"$slug\",\"title\":\"Quickstart smoke\",\"body\":{}}" \
  "$editor/proxy/api/v1/documents"
id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$out")"

expect 404 "a draft is not delivered" \
  -H "Authorization: Bearer $delivery_token" -H "X-Scope-OrgID: $tenant" \
  "$api/delivery/v1/post/$slug"

expect 200 "publish" -b "$jar" -X POST -H "Origin: $editor" \
  "$editor/proxy/api/v1/documents/$id/publish"

expect 200 "delivered once published" \
  -H "Authorization: Bearer $delivery_token" -H "X-Scope-OrgID: $tenant" \
  "$api/delivery/v1/post/$slug"
grep -q 'Quickstart smoke' "$out" || fail "delivery: the document's title is not in the response"

# ── Log out ──────────────────────────────────────────────────────────────────
expect 200 "log out" -b "$jar" -c "$jar" -X POST -H "Origin: $editor" "$editor/auth/logout"
expect 401 "logged out: who am I" -b "$jar" "$editor/auth/me"

echo "quickstart smoke: all steps passed"
