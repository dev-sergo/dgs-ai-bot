#!/usr/bin/env bash
# smoke.sh — детерминированные проверки HTTP-поверхности (Слой 1): auth-гейт + экспорт.
# Не требует рига/LLM: работает с PLANNER_MODE=stub + фикстурами.
#
# Запуск:
#   1) поднять сервер:
#        PLANNER_MODE=stub DGS_CLIENT=fixture AUTH_TOKEN=demo123 HTTP_ADDR=:8099 ./bin/server-host &
#   2) прогнать smoke:
#        BASE=http://localhost:8099 KEY=demo123 test/smoke.sh
#
# Переменные: BASE (default http://localhost:8099), KEY (default demo123),
#             QUERY (default «выручка за последнюю неделю» — есть в фикстурах).
set -u

BASE="${BASE:-http://localhost:8099}"
KEY="${KEY:-demo123}"
QUERY="${QUERY:-выручка за последнюю неделю}"

pass=0; fail=0
ok()  { echo "  ✅ PASS: $1"; pass=$((pass+1)); }
no()  { echo "  ❌ FAIL: $1 — $2"; fail=$((fail+1)); }

code() { # метод url [data] → http-код
  if [ "${1}" = "POST" ]; then
    curl -s -o /dev/null -w '%{http_code}' -X POST "$2" -d "${3:-}"
  else
    curl -s -o /dev/null -w '%{http_code}' "$2"
  fi
}

echo "== smoke @ ${BASE} =="

# 1. /healthz открыт (для health-чека туннеля).
c=$(code GET "${BASE}/healthz")
[ "$c" = "200" ] && ok "healthz открыт (200)" || no "healthz" "got $c, want 200"

# 2. /ask без токена → 401.
c=$(code POST "${BASE}/ask" '{"text":"привет"}')
[ "$c" = "401" ] && ok "гейт блокирует /ask без токена (401)" || no "gate /ask" "got $c, want 401"

# 3. /ask с токеном → 200.
c=$(code POST "${BASE}/ask?key=${KEY}" '{"text":"'"${QUERY}"'"}')
[ "$c" = "200" ] && ok "гейт пускает /ask с токеном (200)" || no "gate /ask+key" "got $c, want 200"

# 4. /ask с токеном в заголовке → 200.
c=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${BASE}/ask" -H "X-Auth-Token: ${KEY}" -d '{"text":"'"${QUERY}"'"}')
[ "$c" = "200" ] && ok "гейт принимает X-Auth-Token (200)" || no "gate header" "got $c, want 200"

# 5. /export без токена → 401.
c=$(code GET "${BASE}/export?text=${QUERY// /%20}")
[ "$c" = "401" ] && ok "гейт блокирует /export без токена (401)" || no "gate /export" "got $c, want 401"

# 6. /export с токеном → .xlsx (магия PK + content-type).
tmp=$(mktemp); hdr=$(mktemp)
curl -s -D "$hdr" -o "$tmp" "${BASE}/export?key=${KEY}&text=${QUERY// /%20}"
ct=$(grep -i '^content-type:' "$hdr" | tr -d '\r')
magic=$(head -c 2 "$tmp")
if echo "$ct" | grep -qi 'spreadsheetml' && [ "$magic" = "PK" ]; then
  ok "экспорт отдаёт валидный .xlsx ($(wc -c <"$tmp" | tr -d ' ') байт)"
else
  no "export xlsx" "content-type=$ct magic=$magic"
fi
rm -f "$tmp" "$hdr"

echo "== итог: ${pass} passed, ${fail} failed =="
[ "$fail" = "0" ]
