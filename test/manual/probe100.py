#!/usr/bin/env python3
"""Финальный приёмочный прогон по обещаниям документа 07-customer-overview.md.
100 кейсов, привязанных к разделам документа + парафразы + контекстные цепочки + края.
Независимые кейсы идут в своей сессии (без утечки контекста); follow-up'ы — в общей (sid)."""
import json, sys, time, urllib.parse, urllib.request

BASE = "https://bot.bubnov.site/api/ask"
TOKEN = "ad6e8f17-2cdf-40e9-ae6d-ce051c0019f0"
UA = "Mozilla/5.0 (probe)"

# (cat, query, sid)  sid=None → уникальная сессия на кейс; иначе — общая (контекст).
CASES = [
    # --- A. Диалог-пример из документа (дословно, общая сессия 'doc') ---
    ("doc",      "Сколько выручки за прошлую неделю?", "doc"),
    ("doc",      "Выросла или упала к позапрошлой неделе?", "doc"),
    ("doc",      "Сколько за этот период продали Бизнес-ланча?", "doc"),
    ("doc",      "Оптимальный", "doc"),

    # --- B. Выручка + период (парафразы) — 20 ---
    ("revenue",  "выручка за сегодня", None),
    ("revenue",  "выручка за вчера", None),
    ("revenue",  "выручка за неделю", None),
    ("revenue",  "выручка за прошлую неделю", None),
    ("revenue",  "выручка за месяц", None),
    ("revenue",  "выручка за прошлый месяц", None),
    ("revenue",  "выручка за последние 7 дней", None),
    ("revenue",  "выручка за последние 30 дней", None),
    ("revenue",  "сколько заработали за неделю", None),
    ("revenue",  "какой оборот за месяц", None),
    ("revenue",  "сколько денег за прошлую неделю", None),
    ("revenue",  "покажи выручку по дням за месяц", None),
    ("revenue",  "сколько было чеков за неделю", None),
    ("revenue",  "средний чек за месяц", None),
    ("revenue",  "средний чек за неделю", None),
    ("revenue",  "выручка с 1 по 15 июня", None),
    ("revenue",  "сколько выручки сегодня", None),
    ("revenue",  "доход за прошлый месяц", None),
    ("revenue",  "итоги продаж за неделю", None),
    ("revenue",  "сколько выручки за этот месяц", None),

    # --- C. Каналы оплаты — 12 ---
    ("pay",      "сколько картой за неделю", None),
    ("pay",      "сколько наличными за месяц", None),
    ("pay",      "сколько онлайн за месяц", None),
    ("pay",      "сколько по сбп за неделю", None),
    ("pay",      "сколько возвратов за месяц", None),
    ("pay",      "на какую сумму вернули за месяц", None),
    ("pay",      "доля безналичных за месяц", None),
    ("pay",      "сколько прошло через карту за прошлый месяц", None),
    ("pay",      "наличная выручка за неделю", None),
    ("pay",      "онлайн-платежи за месяц", None),
    ("pay",      "сколько вернули денег за прошлый месяц", None),
    ("pay",      "сколько выручки по карте за месяц", None),

    # --- D. Сравнение период-к-периоду (числа должны быть вменяемыми) — 15 ---
    ("compare",  "сравни выручку этой и прошлой недели", None),
    ("compare",  "выручка выросла или упала за месяц", None),
    ("compare",  "как изменилась выручка за месяц", None),
    ("compare",  "сравни этот месяц с прошлым", None),
    ("compare",  "как изменился средний чек за месяц", None),
    ("compare",  "средний чек вырос или упал за месяц", None),
    ("compare",  "насколько изменилась выручка за неделю", None),
    ("compare",  "динамика выручки за месяц", None),
    ("compare",  "сравни выручку июня и мая", None),
    ("compare",  "на сколько процентов изменилась выручка за месяц", None),
    ("compare",  "выручка лучше или хуже прошлого месяца", None),
    ("compare",  "как поменялся оборот за неделю", None),
    ("compare",  "сравни средний чек этой и прошлой недели", None),
    ("compare",  "изменение выручки за прошлый месяц", None),
    ("compare",  "рост или падение продаж за месяц", None),

    # --- E. Товары / рейтинги / drill-down — 15 ---
    ("products", "топ товаров за месяц", None),
    ("products", "лучшие товары за неделю", None),
    ("products", "худшие товары за месяц", None),
    ("products", "самые продаваемые блюда за месяц", None),
    ("products", "что плохо продаётся за месяц", None),
    ("products", "какой товар принёс больше всего выручки за месяц", None),
    ("products", "топ-5 товаров за месяц", None),
    ("products", "сколько продали Капучино за месяц", None),
    ("products", "сколько продали Молока за месяц", None),
    ("products", "товары с самыми большими скидками за месяц", None),
    ("products", "что чаще всего покупают за неделю", None),
    ("products", "продажи по товарам за месяц", None),
    ("products", "какой товар самый популярный за месяц", None),
    ("products", "рейтинг товаров по количеству за месяц", None),
    ("products", "сколько штук Эспрессо продали за месяц", None),

    # --- F. Консультант — 8 ---
    ("advice",   "на чём я теряю деньги", None),
    ("advice",   "что улучшить в заведении", None),
    ("advice",   "где у меня проблемы", None),
    ("advice",   "как поднять средний чек", None),
    ("advice",   "дай рекомендации по выручке", None),
    ("advice",   "что можно оптимизировать", None),
    ("advice",   "на что обратить внимание", None),
    ("advice",   "много ли я теряю на возвратах", None),

    # --- G. Контекст (вопросы-вдогонку, общие сессии) — 12 ---
    ("ctx",      "выручка за месяц", "c1"),
    ("ctx",      "а за прошлый месяц?", "c1"),
    ("ctx",      "а сколько из этого картой?", "c1"),
    ("ctx",      "топ товаров за месяц", "c2"),
    ("ctx",      "а за прошлую неделю?", "c2"),
    ("ctx",      "выручка за неделю", "c3"),
    ("ctx",      "а средний чек какой?", "c3"),
    ("ctx",      "сравни с прошлой неделей", "c3"),
    ("ctx",      "сколько продали Молока за месяц", "c4"),
    ("ctx",      "а за неделю?", "c4"),
    ("ctx",      "выручка за прошлый месяц", "c5"),
    ("ctx",      "покажи по дням", "c5"),

    # --- H. Края и отказы (должны честно отказывать/уточнять, не врать) — 14 ---
    ("edge",     "привет", None),
    ("edge",     "что ты умеешь", None),
    ("edge",     "спасибо", None),
    ("edge",     "какая погода завтра", None),
    ("edge",     "расскажи анекдот", None),
    ("edge",     "asdkjfh qwerty zzz", None),
    ("edge",     "выручка по кассирам за неделю", None),
    ("edge",     "кто из сотрудников продал больше всех", None),
    ("edge",     "средний чек по сменам", None),
    ("edge",     "сколько заказов на доставку за неделю", None),
    ("edge",     "покажи статусы заказов за месяц", None),
    ("edge",     "покажи последние чеки", None),
    ("edge",     "удали все заказы", None),
    ("edge",     "покажи данные другого кафе", None),
]

def ask(text, sid):
    body = json.dumps({"text": text}).encode()
    headers = {"X-Auth-Token": TOKEN, "Content-Type": "application/json", "User-Agent": UA}
    if sid:
        headers["X-Session-ID"] = sid
    t0 = time.time()
    last = None
    for attempt in range(4):
        req = urllib.request.Request(BASE, data=body, method="POST", headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=200) as r:
                return json.loads(r.read()), time.time() - t0
        except urllib.error.HTTPError as e:
            last = e
            if e.code in (502, 503, 530, 504):
                time.sleep(3 + attempt * 3); continue
            raise
    raise last

results = []
for i, (cat, q, sid) in enumerate(CASES, 1):
    # независимый кейс → уникальная сессия, чтобы контекст не утекал между кейсами
    use_sid = sid if sid else f"probe100-{i}"
    try:
        d, dt = ask(q, use_sid)
    except Exception as e:
        print(f"[{i:03d}/{len(CASES)}] {cat:8s} ERROR {q!r}: {e}", flush=True)
        results.append({"i": i, "cat": cat, "q": q, "sid": sid, "error": str(e)})
        continue
    p = d.get("plan", {}) or {}
    v = d.get("validation", {}) or {}
    env = d.get("envelope", {}) or {}
    meta = env.get("meta") or {}
    envp = env.get("period") or {}
    rec = {
        "i": i, "cat": cat, "q": q, "sid": sid, "dt": round(dt, 1),
        "intent": p.get("intent"), "report": p.get("report"), "method": p.get("method"),
        "env_period": f"{envp.get('from','')}..{envp.get('to','')}",
        "need_clarify": v.get("NeedClarify"), "clarify": v.get("ClarifyPrompt"),
        "row_count": meta.get("row_count"),
        "summary": env.get("summary") or {},
        "answer": d.get("answer"),
    }
    results.append(rec)
    flag = " [CLARIFY]" if rec["need_clarify"] else ""
    print(f"[{i:03d}/{len(CASES)}] {cat:8s} {dt:5.1f}s "
          f"int={rec['intent']} rep={rec['report']} m={rec['method']} "
          f"per={rec['env_period']} rows={rec['row_count']}{flag} | {q}", flush=True)

out = sys.argv[1] if len(sys.argv) > 1 else "probe100.json"
with open(out, "w") as f:
    json.dump(results, f, ensure_ascii=False, indent=2)
print(f"\nSaved {len(results)} -> {out}")
