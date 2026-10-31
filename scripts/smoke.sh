#!/usr/bin/env sh
set -eu

base_url="${SMOKE_BASE_URL:-http://127.0.0.1:8080}"

echo "GET ${base_url}/healthz"
curl -fsS "${base_url}/healthz" | grep -q '"status":"ok"'

echo "GET ${base_url}/readyz"
curl -fsS "${base_url}/readyz" | grep -q '"status":"ready"'

email="smoke-$(date +%s)@example.com"
password='smoke-password-1'
display='SmokePilot'

echo "POST ${base_url}/v1/auth/register"
reg="$(curl -fsS -X POST "${base_url}/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${email}\",\"password\":\"${password}\",\"display_name\":\"${display}\"}")"
echo "${reg}" | grep -q '"verification_required":true'

dev_token="$(printf '%s' "${reg}" | sed -n 's/.*"dev_verification_token":"\([^"]*\)".*/\1/p')"
if [ -n "${dev_token}" ]; then
  echo "POST ${base_url}/v1/auth/email/verify"
  curl -fsS -X POST "${base_url}/v1/auth/email/verify" \
    -H 'Content-Type: application/json' \
    -d "{\"token\":\"${dev_token}\"}" | grep -q '"ok":true'

  echo "POST ${base_url}/v1/auth/login"
  login="$(curl -fsS -X POST "${base_url}/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"${email}\",\"password\":\"${password}\"}")"
  access="$(printf '%s' "${login}" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"
  if [ -z "${access}" ]; then
    echo "missing access_token in login response" >&2
    exit 1
  fi

  echo "GET ${base_url}/v1/me"
  curl -fsS "${base_url}/v1/me" -H "Authorization: Bearer ${access}" | grep -q '"email_verified":true'
else
  echo "dev_verification_token absent (EMAIL_MODE!=dev); skipped verify/login/me checks"
fi

echo "smoke ok"
