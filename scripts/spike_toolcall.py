#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
spike_toolcall.py — быстрый замер: тянет ли локальная LLM tool-calling (выбор
инструмента + аргументы + заземление сущности + уточнение), ПРЕЖДЕ чем мы
вкладываемся в разворот на оркестратор.

Метод намеренно совпадает с боевым планировщиком: тот же OpenAI-совместимый
эндпоинт llama.cpp, JSON-object режим, temperature 0. Никаких зависимостей
(только stdlib) и никакой Go-сборки — запускается сразу.

Запуск:
  LLM_BASE_URL=http://172.20.10.2:8080 \
  LLM_MODEL=qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07 \
  python3 scripts/spike_toolcall.py

Опционально: LLM_API_KEY, SPIKE_TEMP (по умолчанию 0).
Вывод: по каждому кейсу PASS/FAIL + сырой ответ, в конце — счёт ядра и stretch.
"""

import json
import os
import re
import sys
import urllib.request

BASE_URL = os.environ.get("LLM_BASE_URL", "http://172.20.10.2:8080").rstrip("/")
MODEL    = os.environ.get("LLM_MODEL", "qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07")
API_KEY  = os.environ.get("LLM_API_KEY", "")
TEMP     = float(os.environ.get("SPIKE_TEMP", "0"))

# --- Каталог инструментов (то, что в развороте увидит модель) -----------------
SYSTEM = """Ты — планировщик аналитического ассистента кафе. Тебе доступны ИНСТРУМЕНТЫ.
Твоя задача: разобрать запрос пользователя и вернуть, КАКИЕ инструменты вызвать.
Сам числа НЕ считай — только выбери инструмент(ы) и аргументы. Верни СТРОГО один JSON-объект, без пояснений.

ИНСТРУМЕНТЫ:
1) query_metrics — получить метрику из отчётной системы.
   args: {
     "metric": один из ["revenue","orders_count","checks_count","avg_check","payback","profit"],
     "group_by": список из ["product","category","payment_type","order_type","source","sale_point","region","day","month"] (можно пусто),
     "filters": объект, ключи из ["sale_point","region","payment_type","order_type","source","product","category"];
                payment_type ∈ card|cash|online|sbp; order_type ∈ delivery|dinein|pickup,
     "period": строка (например "вчера", "этот месяц", "июнь", "эта неделя vs прошлая"),
     "sort": "desc"|"asc" (опц.), "limit": число (опц.)
   }
2) resolve_entity — сопоставить человеческое имя реальному объекту каталога ПЕРЕД фильтрацией.
   args: { "name": строка, "kind": один из ["sale_point","product","category","employee"] }
3) forecast_revenue — прогноз выручки (детерминированный темп), опц. сравнение с целью.
   args: { "period": строка, "goal": число (опц.) }
4) clarify — если запрос неоднозначен или не хватает данных (несколько вариантов товара, неясен период).
   args: { "question": строка }
5) none — болтовня / приветствие / благодарность / не по теме.
   args: { "reply": строка }

ПРАВИЛА:
- Если в запросе есть НАЗВАНИЕ точки/товара/сотрудника — сначала шаг resolve_entity, потом query_metrics.
- Если запрос неоднозначен (например товар с несколькими вариантами) — верни clarify, не угадывай.
- Можно вернуть НЕСКОЛЬКО шагов (например «почему упала выручка» → несколько query_metrics по разным разрезам).

ФОРМАТ ОТВЕТА (строго один из):
{"steps":[{"tool":"<имя>","args":{...}}, ...]}
"""

# --- Кейсы: query, ядро/stretch, ожидание ------------------------------------
# check: {"tool": ожид. инструмент 1-го шага | "any" со списком}, "must": [подстроки в склейке args],
#        "alt_tools": допустимые альтернативы (для clarify/resolve), "min_qm": мин. число query_metrics (stretch)
CASES = [
    {"q": "Привет, как вчера по выручке?", "core": True,
     "tool": "query_metrics", "must": ["revenue"]},
    {"q": "выручка наличкой по Выксе за июнь", "core": True,
     "any": ["query_metrics", "resolve_entity"], "must": ["revenue", "cash", "выкс"]},
    {"q": "сравни выручку этой недели с прошлой", "core": True,
     "tool": "query_metrics", "must": ["revenue", "недел"]},
    {"q": "топ-5 товаров за месяц по продажам", "core": True,
     "tool": "query_metrics", "must": ["product"]},
    {"q": "какой средний чек по доставке?", "core": True,
     "tool": "query_metrics", "must": ["avg_check", "delivery"]},
    {"q": "дойду ли я до плана в 2 миллиона в этом месяце?", "core": True,
     "tool": "forecast_revenue", "must": ["2"]},
    {"q": "сколько продали Бизнес-ланча?", "core": True,
     "any": ["clarify", "resolve_entity"], "must": []},
    {"q": "спасибо большое, всё понятно!", "core": True,
     "tool": "none", "must": []},
    {"q": "какая у меня структура продаж?", "core": True,
     "tool": "query_metrics", "must": []},  # group_by по каналу/типу — смотрим глазами
    {"q": "почему за последние 2 недели упала выручка?", "core": False,
     "min_qm": 2, "must": ["revenue"]},  # stretch: многошаговый разбор
]


def chat(query):
    body = {
        "model": MODEL,
        "messages": [{"role": "system", "content": SYSTEM},
                     {"role": "user", "content": query}],
        "temperature": TEMP,
        "max_tokens": 512,
        "response_format": {"type": "json_object"},
    }
    data = json.dumps(body).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if API_KEY:
        headers["Authorization"] = "Bearer " + API_KEY
    req = urllib.request.Request(BASE_URL + "/v1/chat/completions", data=data, headers=headers)
    with urllib.request.urlopen(req, timeout=180) as r:
        resp = json.load(r)
    return resp["choices"][0]["message"]["content"]


def parse_steps(raw):
    """Достаём список шагов; терпимы к тому, что модель вернёт чуть иначе."""
    txt = raw.strip()
    m = re.search(r"\{.*\}", txt, re.S)
    if m:
        txt = m.group(0)
    obj = json.loads(txt)  # бросит — поймаем выше как parse-fail
    if isinstance(obj, dict) and "steps" in obj:
        steps = obj["steps"]
    elif isinstance(obj, dict) and "tool" in obj:
        steps = [obj]
    elif isinstance(obj, list):
        steps = obj
    else:
        steps = [obj]
    return steps


def score(case, steps):
    if not steps:
        return False, "пустой план"
    tools = [str(s.get("tool", "")) for s in steps]
    first = tools[0]
    blob = json.dumps(steps, ensure_ascii=False).lower()
    must_ok = all(s.lower() in blob for s in case.get("must", []))

    if "min_qm" in case:  # stretch
        n_qm = sum(1 for t in tools if t == "query_metrics")
        ok = n_qm >= case["min_qm"] and must_ok
        return ok, f"query_metrics×{n_qm} (нужно ≥{case['min_qm']}), must={must_ok}"

    if "any" in case:
        tool_ok = any(t in case["any"] for t in tools)
        return tool_ok and must_ok, f"tools={tools} ∈ {case['any']}? {tool_ok}, must={must_ok}"

    tool_ok = first == case["tool"]
    return tool_ok and must_ok, f"first={first} (нужно {case['tool']}), must={must_ok}"


def main():
    print(f"spike tool-calling | model={MODEL} | temp={TEMP} | {BASE_URL}\n" + "=" * 70)
    core_pass = core_total = 0
    stretch = []
    for i, c in enumerate(CASES, 1):
        try:
            raw = chat(c["q"])
        except Exception as e:
            print(f"[{i}] ✗ HTTP/LLM error: {e}\n    q: {c['q']}")
            if c.get("core"):
                core_total += 1
            continue
        try:
            steps = parse_steps(raw)
            ok, why = score(c, steps)
        except Exception as e:
            ok, why, steps = False, f"PARSE-FAIL: {e}", None
        tag = "ядро " if c.get("core") else "strch"
        mark = "✓" if ok else "✗"
        print(f"[{i}] {mark} ({tag}) {c['q']}")
        print(f"      → {why}")
        print(f"      raw: {raw.strip()[:300]}")
        if c.get("core"):
            core_total += 1
            core_pass += 1 if ok else 0
        else:
            stretch.append(ok)
    print("=" * 70)
    print(f"ЯДРО:   {core_pass}/{core_total}  (бар прохождения: ≥ {max(1, core_total-1)}/{core_total})")
    if stretch:
        print(f"STRETCH (многошаг): {sum(stretch)}/{len(stretch)} — приятно, но не обязательно для go")
    print("\nКритерий go: ядро пройдено + сущности заземляются + неоднозначное → clarify + 'спасибо' → none.")


if __name__ == "__main__":
    sys.exit(main())
