#!/usr/bin/env python3
"""Проверка экспорта в Excel на живом стенде: GET /api/export?text=...
Валидирует: HTTP-статус, content-type, что файл — настоящий xlsx (zip + xl/workbook.xml),
и что внутри есть русское содержимое (sharedStrings). Краевые случаи — что отдаёт 4xx."""
import io, json, sys, time, urllib.parse, urllib.request, zipfile

BASE = "https://bot.bubnov.site/api/export"
TOKEN = "ad6e8f17-2cdf-40e9-ae6d-ce051c0019f0"
UA = "Mozilla/5.0 (probe)"

# (query, ожидание): "xlsx" — должен скачаться валидный файл; "reject" — должен отказать (4xx).
CASES = [
    ("выручка за месяц", "xlsx"),
    ("выручка по дням за месяц", "xlsx"),
    ("топ товаров за месяц", "xlsx"),
    ("худшие товары за месяц", "xlsx"),
    ("сколько оплат картой за неделю", "xlsx"),
    ("что просело по сравнению с прошлой неделей", "xlsx"),  # contribution — есть строки
    ("выручка за сегодня", "reject"),       # нет данных
    ("сравни выручку этой и прошлой недели", "reject"),  # compare — нет строк
    ("привет", "reject"),                   # не отчёт
]

def fetch(text):
    url = BASE + "?text=" + urllib.parse.quote(text)
    req = urllib.request.Request(url, headers={"X-Auth-Token": TOKEN, "User-Agent": UA})
    t0 = time.time()
    for attempt in range(4):
        try:
            r = urllib.request.urlopen(req, timeout=120)
            return r.status, r.headers.get("Content-Type", ""), r.read(), time.time() - t0
        except urllib.error.HTTPError as e:
            if e.code in (502, 503, 530, 504):
                time.sleep(3 + attempt * 3); continue
            return e.code, e.headers.get("Content-Type", ""), e.read(), time.time() - t0
    return 0, "", b"", time.time() - t0

def validate_xlsx(blob):
    """Возвращает (ok, описание): валидный ли xlsx и что внутри."""
    if blob[:4] != b"PK\x03\x04":
        return False, "не zip/xlsx (нет PK-сигнатуры)"
    try:
        z = zipfile.ZipFile(io.BytesIO(blob))
    except zipfile.BadZipFile as e:
        return False, f"битый zip: {e}"
    names = z.namelist()
    if "xl/workbook.xml" not in names:
        return False, f"нет xl/workbook.xml; есть: {names[:5]}"
    bad = z.testzip()
    if bad is not None:
        return False, f"CRC-ошибка в {bad}"
    # содержимое строк (заголовки/значения)
    shared = ""
    if "xl/sharedStrings.xml" in names:
        shared = z.read("xl/sharedStrings.xml").decode("utf-8", "replace")
    cyr = any("а" <= ch.lower() <= "я" for ch in shared)
    n_strings = shared.count("<t>") + shared.count("<t ")
    return True, f"zip ок, {len(names)} частей, строк≈{n_strings}, кириллица={'да' if cyr else 'нет'}"

passed = 0
for q, expect in CASES:
    code, ctype, blob, dt = fetch(q)
    if expect == "xlsx":
        ok, desc = validate_xlsx(blob) if code == 200 else (False, f"HTTP {code}")
        verdict = "PASS" if ok else "FAIL"
        extra = f"{len(blob)}b {dt:.1f}s | {desc}"
    else:  # reject
        is_reject = code in (400, 415, 422) and "spreadsheet" not in ctype
        ok = is_reject
        verdict = "PASS" if ok else "FAIL"
        body = blob.decode("utf-8", "replace")[:120].replace("\n", " ")
        extra = f"HTTP {code} {ctype.split(';')[0]} | {body}"
    passed += ok
    print(f"[{verdict}] expect={expect:6s} {q!r}\n        {extra}")

print(f"\n{passed}/{len(CASES)} прошло")
