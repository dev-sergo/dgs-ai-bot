#!/usr/bin/env python3
"""
Нормализует сырые сетки (*.grid.json) -> чистые фикстуры <slug>.json + catalog.example.yaml,
ВЫРЕЗАЯ секреты и PII (email/телефон/пин/ИНН/ФИО/адрес/координаты/дни рождения).
Также: маскирует grid.json на месте, чистит _index.json от токенов, перегенерит _structure.md,
собирает tenants.example.json. Сырой HTML (raw/) удаляется отдельно (в нём живые секреты).

Запуск:  python3 tools/normalize_fixtures.py
"""
import os, re, json, glob

ROOT = "docs/contracts/fixtures"
DIRS = [ROOT, f"{ROOT}/entities"]

_TR = {'а':'a','б':'b','в':'v','г':'g','д':'d','е':'e','ё':'e','ж':'zh','з':'z','и':'i',
       'й':'y','к':'k','л':'l','м':'m','н':'n','о':'o','п':'p','р':'r','с':'s','т':'t',
       'у':'u','ф':'f','х':'h','ц':'c','ч':'ch','ш':'sh','щ':'sch','ъ':'','ы':'y','ь':'',
       'э':'e','ю':'yu','я':'ya'}

def slug(label):
    s = "".join(_TR.get(c, c) for c in (label or "").lower())
    return re.sub(r'[^a-z0-9]+', '_', s).strip('_') or "col"

RE_NUM  = re.compile(r'^-?\d[\d \xa0]*(?:[.,]\d+)?\s*₽?$')
RE_PCT  = re.compile(r'^-?\d[\d \xa0]*(?:[.,]\d+)?\s*%$')
RE_DATE = re.compile(r'^(\d{2})\.(\d{2})\.(\d{4})$')

def norm(cell):
    t = (cell or "").replace('\xa0', ' ').strip()
    if t in ("", "—", "Ничего не найдено."):
        return None
    m = RE_DATE.match(t)
    if m:
        return f"{m.group(3)}-{m.group(2)}-{m.group(1)}"
    if RE_NUM.match(t) or RE_PCT.match(t):
        v = t.rstrip('%₽ ').replace(' ', '').replace(',', '.')
        try:
            return float(v)
        except ValueError:
            pass
    return t

# ---- PII / секреты -------------------------------------------------------
def pii_kind(label, key):
    k = (key or '').lower(); l = (label or '').lower()
    if any(w in l for w in ('кол-во', 'количество', 'сумма', 'уровн', 'выручк', 'прибыл')):
        return None  # агрегаты — не PII
    if k in ('email',) or any(w in l for w in ('e-mail', 'email', 'почта')):     return 'email'
    if k in ('phone',) or any(w in l for w in ('телефон', 'phone', 'факс', 'fax')): return 'phone'
    if k == 'pin' or 'пин' in l:                                                  return 'pin'
    if 'инн' in l:                                                                return 'inn'
    if 'день рождения' in l or 'birth' in l:                                      return 'null'
    if 'координат' in l:                                                          return 'null'
    if 'адрес' in l or 'address' in l:                                            return 'mask'
    if k in ('cashier_name', 'user_name', 'first_name', 'last_name', 'owner'):    return 'mask'
    if any(w in l for w in ('фио', 'владелец', 'кассир', 'сотрудник',
                            'имя покупател', 'покупатель', 'водител', 'создал', 'обновил')):
        return 'mask'
    return None

def redact(val, kind):
    if val in (None, ""):
        return None
    return {'email': "redacted@example.com", 'phone': "+70000000000", 'pin': "0000",
            'inn': "0000000000", 'null': None, 'mask': "REDACTED"}[kind]

def unit_of(label, field):
    l = (label or "").lower()
    if '₽' in label: return "RUB"
    if '%' in label: return "percent"
    if 'дата' in l or field in ("date", "close", "open"): return "date"
    if 'кол-во' in l or 'количество' in l or (field or '').startswith('quantity'): return "count"
    return "string"

# ---- основной проход -----------------------------------------------------
def process(gp):
    g = json.load(open(gp))
    cols = g["columns"]
    keys, kinds, seen = [], [], {}
    for c in cols:
        k = (c.get("field") or "").lstrip('-') or slug(c["label"])
        if k in seen:
            seen[k] += 1; k = f"{k}_{seen[k]}"
        else:
            seen[k] = 0
        keys.append(k); kinds.append(pii_kind(c["label"], c.get("field")))
    records = []
    for row in g["rows"]:
        cells = row["cells"] if isinstance(row, dict) else row
        rec = {}
        for i in range(min(len(keys), len(cells))):
            rec[keys[i]] = redact(cells[i], kinds[i]) if kinds[i] else norm(cells[i])
        records.append(rec)
    # перезаписываем grid.json уже без PII (маскируем исходные ячейки)
    for row in g["rows"]:
        cells = row["cells"] if isinstance(row, dict) else row
        for i in range(min(len(keys), len(cells))):
            if kinds[i]:
                cells[i] = "" if redact(cells[i], kinds[i]) is None else str(redact(cells[i], kinds[i]))
    json.dump(g, open(gp, "w"), ensure_ascii=False, indent=2)
    return keys, kinds, cols, records, g.get("label", "")

def main():
    catalog = ["# Каталог отчётов и сущностей Dooglys (автоген, PII вырезан)", ""]
    structure = ["# Структура данных Dooglys (нормализовано, PII вырезан)", ""]
    redacted_cols = 0
    for d in DIRS:
        section = "ОТЧЁТЫ" if d == ROOT else "СУЩНОСТИ/СПРАВОЧНИКИ"
        catalog.append(f"# === {section} ===")
        for gp in sorted(glob.glob(f"{d}/*.grid.json")):
            s = os.path.basename(gp)[:-len(".grid.json")]
            keys, kinds, cols, records, label = process(gp)
            json.dump({"report": s, "label": label, "rows": records},
                      open(f"{d}/{s}.json", "w"), ensure_ascii=False, indent=2)
            redacted_cols += sum(1 for k in kinds if k)
            catalog.append(f"{s}:")
            catalog.append(f"  name: \"{label or s}\"")
            catalog.append("  fields:")
            for k, c, kind in zip(keys, cols, kinds):
                if not c["label"] and not c.get("field"):
                    continue
                pii = "  pii: true" if kind else ""
                catalog.append(f"    - {{key: {k}, label: \"{c['label']}\", unit: {unit_of(c['label'], c.get('field'))}{', pii: true' if kind else ''}}}")
            catalog.append("")
            structure.append(f"## {label or s} — {s}  (строк: {len(records)})")
            if records:
                structure.append("```json")
                structure.append(json.dumps(records[0], ensure_ascii=False))
                structure.append("```")
            structure.append("")
    open(f"{ROOT}/catalog.example.yaml", "w").write("\n".join(catalog))
    open(f"{ROOT}/_structure.md", "w").write("\n".join(structure))

    # чистим _index.json от токенов
    ip = f"{ROOT}/_index.json"
    if os.path.exists(ip):
        idx = json.load(open(ip))
        for sk in ("access-token", "csrf-token"):
            idx.get("meta", {}).pop(sk, None)
        json.dump(idx, open(ip, "w"), ensure_ascii=False, indent=2)

    # mock-тенанты (имена точек/регионов — бизнес-данные, не PII; id синтетические)
    tenants = [
        {"tenant_id": "11111111-0000-0000-0000-000000000001", "domain": "mock_single",
         "timezone": "Europe/Moscow", "currency": "RUB", "currency_precision": 2,
         "sale_points": [{"id": "aaaa0001-0000-0000-0000-000000000001", "name": "Кафе на Садовой",
                          "locality": "Москва"}]},
        {"tenant_id": "11111111-0000-0000-0000-000000000002", "domain": "mock_multi",
         "timezone": "Europe/Moscow", "currency": "RUB", "currency_precision": 2,
         "sale_points": [{"id": "aaaa0002-0000-0000-0000-000000000001", "name": "Точка НН", "locality": "НН"},
                         {"id": "aaaa0002-0000-0000-0000-000000000002", "name": "Точка Казань", "locality": "Казань"},
                         {"id": "aaaa0002-0000-0000-0000-000000000003", "name": "Точка СПБ", "locality": "СПБ"}]},
        {"tenant_id": "11111111-0000-0000-0000-000000000003", "domain": "mock_tz",
         "timezone": "Asia/Yekaterinburg", "currency": "RUB", "currency_precision": 2,
         "sale_points": [{"id": "aaaa0003-0000-0000-0000-000000000001", "name": "Точка Самара", "locality": "Самара"}]},
    ]
    json.dump({"tenants": tenants}, open(f"{ROOT}/tenants.example.json", "w"),
              ensure_ascii=False, indent=2)

    print(f"Нормализовано фикстур: {len(glob.glob(ROOT+'/*.json')) + len(glob.glob(ROOT+'/entities/*.json'))}")
    print(f"Замаскировано PII-колонок: {redacted_cols}")
    print("Готово: <slug>.json, catalog.example.yaml, _structure.md, tenants.example.json")
    print("grid.json перезаписаны без PII, _index.json очищен от токенов.")

if __name__ == "__main__":
    main()
