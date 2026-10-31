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

  if [ -n "${DEDICATED_SERVER_CREDENTIAL:-}" ]; then
    echo "POST ${base_url}/v1/server/instances/register"
    reg_inst="$(curl -fsS -X POST "${base_url}/v1/server/instances/register" \
      -H "Authorization: Bearer ${DEDICATED_SERVER_CREDENTIAL}" \
      -H 'Content-Type: application/json' \
      -d '{"host":"127.0.0.1","port":24567,"transport":"enet_udp","protocol_version":1,"server_build":"smoke","capacity":4,"status":"starting"}')"
    echo "${reg_inst}" | grep -q '"game_instance_id"'
    instance_id="$(printf '%s' "${reg_inst}" | sed -n 's/.*"game_instance_id":"\([^"]*\)".*/\1/p')"

    echo "POST ${base_url}/v1/server/instances/${instance_id}/heartbeat"
    curl -fsS -X POST "${base_url}/v1/server/instances/${instance_id}/heartbeat" \
      -H "Authorization: Bearer ${DEDICATED_SERVER_CREDENTIAL}" \
      -H 'Content-Type: application/json' \
      -d '{"status":"ready"}' | grep -q '"ok":true'

    echo "POST ${base_url}/v1/games/assign"
    asg="$(curl -fsS -X POST "${base_url}/v1/games/assign" \
      -H "Authorization: Bearer ${access}" \
      -H 'Content-Type: application/json' \
      -d '{"mode":"coop_survival"}')"
    echo "${asg}" | grep -q '"game_ticket"'
  else
    echo "DEDICATED_SERVER_CREDENTIAL unset; skipped Phase 2B DS/assign smoke"
  fi
else
  echo "dev_verification_token absent (EMAIL_MODE!=dev); skipped verify/login/me checks"
fi

echo "smoke ok"
