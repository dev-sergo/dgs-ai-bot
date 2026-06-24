#!/usr/bin/env python3
"""Ad-hoc probe of the live stand: send ~50 common queries, capture plan + result."""
import json, sys, time, urllib.request

BASE = "https://bot.bubnov.site/api/ask"
TOKEN = "ad6e8f17-2cdf-40e9-ae6d-ce051c0019f0"

QUERIES = [
    # --- Выручка / период ---
    ("revenue", "выручка за сегодня"),
    ("revenue", "выручка за вчера"),
    ("revenue", "выручка за неделю"),
    ("revenue", "выручка за прошлую неделю"),
    ("revenue", "выручка за месяц"),
    ("revenue", "выручка за прошлый месяц"),
    ("revenue", "сколько заработали за последние 7 дней"),
    ("revenue", "покажи выручку по дням за месяц"),
    ("revenue", "сколько было чеков на этой неделе"),
    ("revenue", "какой средний чек за месяц"),
    # --- Каналы оплаты ---
    ("pay", "сколько оплат картой за неделю"),
    ("pay", "сколько наличными за месяц"),
    ("pay", "сколько через сбп за неделю"),
    ("pay", "какая доля онлайн-оплат за месяц"),
    ("pay", "сколько возвратов за месяц"),
    ("pay", "на какую сумму вернули за неделю"),
    # --- Сравнения / тренды ---
    ("compare", "сравни выручку этой и прошлой недели"),
    ("compare", "выручка выросла или упала по сравнению с прошлым месяцем"),
    ("compare", "как изменился средний чек за месяц"),
    ("compare", "что просело по сравнению с прошлой неделей"),
    ("compare", "почему упала выручка"),
    ("compare", "сравни этот месяц с прошлым"),
    # --- Товары ---
    ("products", "топ товаров за месяц"),
    ("products", "какие товары продавались лучше всего"),
    ("products", "худшие товары за месяц"),
    ("products", "сколько продали кофе за неделю"),
    ("products", "покажи продажи по категориям"),
    ("products", "какой товар принёс больше всего выручки"),
    ("products", "сколько порций бизнес-ланча продали за месяц"),
    ("products", "товары с самыми большими скидками"),
    ("products", "что чаще всего возвращают"),
    # --- Заказы / чеки ---
    ("orders", "сколько заказов на доставку за неделю"),
    ("orders", "сколько самовывозов за месяц"),
    ("orders", "какие источники заказов"),
    ("orders", "покажи статусы заказов"),
    ("orders", "средний чек по доставке"),
    ("receipts", "сколько чеков за сегодня"),
    ("receipts", "покажи последние чеки"),
    # --- Консультант / совет ---
    ("advice", "на чём я теряю деньги"),
    ("advice", "что улучшить в заведении"),
    ("advice", "дай рекомендации по выручке"),
    ("advice", "как поднять средний чек"),
    ("advice", "где у меня проблемы"),
    # --- Персонал / смены (пока не на живых данных) ---
    ("staff", "выручка по кассирам за неделю"),
    ("staff", "кто из сотрудников продал больше всего"),
    ("staff", "средний чек по сменам"),
    # --- Диалог / край / мусор ---
    ("meta", "привет"),
    ("meta", "что ты умеешь"),
    ("meta", "какая погода завтра"),
    ("meta", "asdkjfh qwerty zzz"),
]

UA = "Mozilla/5.0 (probe)"  # WAF blocks default python-urllib UA

def ask(text):
    body = json.dumps({"text": text}).encode()
    t0 = time.time()
    last = None
    for attempt in range(4):
        req = urllib.request.Request(BASE, data=body, method="POST",
            headers={"X-Auth-Token": TOKEN, "Content-Type": "application/json",
                     "User-Agent": UA})
        try:
            with urllib.request.urlopen(req, timeout=200) as r:
                return json.loads(r.read()), time.time() - t0
        except urllib.error.HTTPError as e:
            last = e
            if e.code in (502, 503, 530, 504):  # transient Cloudflare/origin
                time.sleep(3 + attempt * 3)
                continue
            raise
    raise last

results = []
for i, (cat, q) in enumerate(QUERIES, 1):
    try:
        d, dt = ask(q)
    except Exception as e:
        print(f"[{i:02d}/{len(QUERIES)}] {cat:9s} ERROR {q!r}: {e}", flush=True)
        results.append({"cat": cat, "q": q, "error": str(e)})
        continue
    p = d.get("plan", {}) or {}
    v = d.get("validation", {}) or {}
    env = d.get("envelope", {}) or {}
    period = (p.get("period") or {})
    envperiod = (env.get("period") or {})
    summary = env.get("summary") or {}
    meta = env.get("meta") or {}
    rec = {
        "cat": cat, "q": q, "dt": round(dt, 1),
        "intent": p.get("intent"), "report": p.get("report"),
        "method": p.get("method"), "class": p.get("class"),
        "period_tok": period.get("token") or period.get("kind"),
        "env_period": f"{envperiod.get('from','')}..{envperiod.get('to','')}",
        "conf": p.get("confidence"),
        "need_clarify": v.get("NeedClarify"),
        "clarify": v.get("ClarifyPrompt"),
        "row_count": meta.get("row_count"),
        "summary": summary,
        "answer": d.get("answer"),
    }
    results.append(rec)
    flag = ""
    if rec["need_clarify"]: flag = " [CLARIFY]"
    if "error" in rec: flag = " [ERROR]"
    print(f"[{i:02d}/{len(QUERIES)}] {cat:9s} {dt:5.1f}s "
          f"intent={rec['intent']} report={rec['report']} method={rec['method']} "
          f"period={rec['env_period']} rows={rec['row_count']}{flag}  | {q}", flush=True)

with open(sys.argv[1] if len(sys.argv) > 1 else "probe50.json", "w") as f:
    json.dump(results, f, ensure_ascii=False, indent=2)
print(f"\nSaved {len(results)} results -> {sys.argv[1] if len(sys.argv)>1 else 'probe50.json'}")
