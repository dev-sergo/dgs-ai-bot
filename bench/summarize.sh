#!/usr/bin/env bash
# bench/summarize.sh <run.log> — печатает JSON-сводку одного прогона.
#
# Метаданные берутся из ИМЕНИ файла, которое обязано следовать контракту:
#   <YYYY-MM-DD>_<HHMM>_<harness>_<corpus>_<mode>_<commit>.log
# Метрики (pass/total/валидные/латентность/модель) парсятся из тела лога.
#
# Использование:
#   bench/summarize.sh bench/runs/2026-06-26_1044_eval_prompts_llm_f705bb7.log
#   for f in bench/runs/*.log; do bench/summarize.sh "$f" > "${f%.log}.json"; done
set -euo pipefail

f="${1:?usage: summarize.sh <run.log>}"
base="$(basename "$f")"; base="${base%.log}"
IFS='_' read -r d t harness corpus mode commit <<<"$base"

body="$(cat "$f")"

# First occurrence = the run's own итог line (guards against logs with appended blocks).
pt="$(grep -oE '(прошло|Итог):?[[:space:]]*[0-9]+/[0-9]+' <<<"$body" | head -1 | grep -oE '[0-9]+/[0-9]+' || true)"
pass="${pt%/*}"; total="${pt#*/}"
valid="$(grep -oE 'валидных:?[[:space:]]*[0-9]+' <<<"$body" | head -1 | grep -oE '[0-9]+$' || true)"
p50="$(grep -oE 'p50=[0-9]+' <<<"$body" | head -1 | sed 's/p50=//' || true)"
p95="$(grep -oE 'p95=[0-9]+' <<<"$body" | head -1 | sed 's/p95=//' || true)"
model="$(grep -oE 'модель=[^,]+' <<<"$body" | head -1 | sed 's/модель=//' || true)"

rate=null
if [[ -n "${pass:-}" && -n "${total:-}" && "${total:-0}" -gt 0 ]]; then
  rate="$(awk "BEGIN{printf \"%.3f\", $pass/$total}")"
fi

printf '{"date":"%s","time":"%s","harness":"%s","corpus":"%s","mode":"%s","commit":"%s","pass":%s,"total":%s,"valid":%s,"rate":%s,"p50_ms":%s,"p95_ms":%s,"model":"%s"}\n' \
  "$d" "$t" "$harness" "$corpus" "$mode" "${commit:-unknown}" \
  "${pass:-null}" "${total:-null}" "${valid:-null}" "$rate" "${p50:-null}" "${p95:-null}" "${model:-}"
