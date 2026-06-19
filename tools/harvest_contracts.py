#!/usr/bin/env python3
"""
Харвестер контрактов Dooglys: за один проход обходит каталог отчётов,
сохраняет сырой HTML и извлекает GridView-таблицу (поля + значения) в фикстуры.
Плюс разведывает JSON-API из фронтового бандла report/index.js.

Запуск:
    export DGS_COOKIE='advanced-backend=...; _csrf-backend=...; _identity-backend=...'
    python3 tools/harvest_contracts.py

Cookie живёт ~1 час (Max-Age=3600 у _identity-backend) — запускай сразу после логина.
"""
import os, sys, json, gzip, time, re
import urllib.request
from html.parser import HTMLParser

BASE   = os.environ.get("DGS_BASE", "https://google.dooglys.com")
COOKIE = os.environ.get("DGS_COOKIE", "")
PERIOD = os.environ.get("DGS_PERIOD", "01.01.2026-19.06.2026")   # DD.MM.YYYY-DD.MM.YYYY
OUT    = os.environ.get("DGS_OUT", "docs/contracts/fixtures")
DELAY  = float(os.environ.get("DGS_DELAY", "0.7"))

if not COOKIE:
    sys.exit("ОШИБКА: задай переменную DGS_COOKIE (полный заголовок Cookie из браузера)")

# Каталог отчётов: slug -> человекочитаемое имя (подтверждено пользователем)
REPORTS = {
    "payment": "Выручка",
    "expected-profit": "Ожидаемая прибыль",
    "orders": "Заказы",
    "active-orders": "Активные заказы",
    "paycheck": "Чеки",
    "cash-on-hand": "Наличные",
    "cash-income-outcome": "Внесения и выплаты",
    "products": "Товары",
    "categories": "Категории",
    "personnel": "Персонал",
    "clients": "Клиенты",
    "sales-on-map": "Продажи на карте",
    "abc": "ABC",
    "source-order": "Источники заказов",
    "order-payment-hour": "Часы заказов",
    "order-processing-time": "Время выполнения",
    "special": "Акции",
    "special-products": "Товары по акции",
    "level-balance": "Балансы по уровням",
    "warehouse-document-supply": "Поступления (склад)",
    "stock": "Остатки по складам",
    "product-flow": "Отчёт по движению",
    "minimum-stock": "Минимальные остатки",
    "write-off-report": "Отчёт по списаниям",
    "purchase-price-dynamics": "Динамика закупочных цен",
}

# Страницы для модели тенанта (TZ, точки, оргструктура)
PAGES = {
    "tenant-settings": "/structure/tenant-settings",
    "sale-point":      "/structure/sale-point",
    "organization":    "/structure/organization",
}

HDRS = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:151.0) Gecko/20100101 Firefox/151.0",
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    "Accept-Language": "ru-RU,ru;q=0.9",
    "Accept-Encoding": "gzip",
    "Cookie": COOKIE,
}


def fetch(path):
    url = path if path.startswith("http") else BASE + path
    req = urllib.request.Request(url, headers=HDRS)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            data = r.read()
            if r.headers.get("Content-Encoding") == "gzip":
                data = gzip.decompress(data)
            return r.status, data.decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, ""
    except Exception as e:
        return 0, f"__ERR__ {e}"


class Grid(HTMLParser):
    """Извлекает первую таблицу .table-wrap: колонки(+data-sort), строки(+data-key), итог."""
    def __init__(self):
        super().__init__()
        self.in_tbl = False; self.sect = None
        self.in_cell = False; self.cell = ""
        self.row = []; self.rowmeta = {}; self.cellmeta = {}
        self.cols = []; self.rows = []; self.foot = []

    def handle_starttag(self, t, a):
        ad = dict(a)
        if t == "table":
            self.in_tbl = True
        if not self.in_tbl:
            return
        if t in ("thead", "tbody", "tfoot"):
            self.sect = t
        elif t == "tr":
            self.row = []; self.rowmeta = {"key": ad.get("data-key")}
        elif t in ("td", "th"):
            self.in_cell = True; self.cell = ""
            self.cellmeta = {"field": ad.get("data-sort")}
        elif t == "a" and self.in_cell:
            if ad.get("data-sort"):
                self.cellmeta["field"] = ad["data-sort"]
            if ad.get("data-cash-on-hand-url"):
                self.rowmeta["detail_url"] = ad["data-cash-on-hand-url"]

    def handle_data(self, d):
        if self.in_cell:
            self.cell += d

    def handle_endtag(self, t):
        if not self.in_tbl:
            return
        if t in ("td", "th"):
            self.in_cell = False
            txt = " ".join(self.cell.split())
            if self.sect == "thead":
                self.cols.append({"label": txt, "field": self.cellmeta.get("field")})
            else:
                self.row.append(txt)
        elif t == "tr":
            if self.sect == "tbody" and self.row:
                self.rows.append({"meta": self.rowmeta, "cells": self.row})
            elif self.sect == "tfoot" and self.row:
                self.foot = self.row
        elif t == "table":
            self.in_tbl = False


def grid_chunk(html):
    """Вырезает блок с дата-таблицей, чтобы не цеплять layout-таблицы."""
    for pat in (r'<div[^>]*class="[^"]*grid-view[^"]*".*?</table>',
                r'<div[^>]*id="reports".*?</table>',
                r'<div[^>]*class="[^"]*report-content[^"]*".*?</table>',
                r'<div[^>]*class="[^"]*table__wrapper[^"]*".*?</table>'):
        m = re.search(pat, html, re.S)
        if m:
            return m.group(0)
    return html


def head_meta(html):
    keys = ["tenant-id", "tenant-domain", "currency-code", "currency-symbol",
            "access-token", "csrf-token", "loyalty-type"]
    out = {}
    for k in keys:
        m = re.search(r'name="%s"\s+content="([^"]*)"' % re.escape(k), html)
        if m:
            out[k] = m.group(1)
    return out


def main():
    os.makedirs(os.path.join(OUT, "raw"), exist_ok=True)
    df, dt = PERIOD.split("-")
    q = "?BaseReportForm[period]=%s" % PERIOD
    index = {"period": PERIOD, "reports": {}, "pages": {}, "meta": {}, "json_api": {}}

    # 1) Обход отчётов: SSR-сетка
    items = list(REPORTS.items())
    for i, (slug, label) in enumerate(items, 1):
        status, html = fetch("/report/%s%s" % (slug, q))
        print("[%2d/%d] %-28s HTTP %s  %d B" % (i, len(items), slug, status, len(html)))
        with open(os.path.join(OUT, "raw", "%s.html" % slug), "w") as f:
            f.write(html)
        has_login = "site/login" in html or "LoginForm" in html
        has_grid = ("grid-view" in html or "table-wrap" in html)
        if status == 200 and has_grid and not has_login:
            g = Grid(); g.feed(grid_chunk(html))
            rec = {"label": label, "http": status, "columns": g.cols,
                   "rows": g.rows, "total": g.foot, "row_count": len(g.rows)}
            with open(os.path.join(OUT, "%s.grid.json" % slug), "w") as f:
                json.dump(rec, f, ensure_ascii=False, indent=2)
            index["reports"][slug] = {"label": label, "http": status,
                                      "cols": [c["label"] for c in g.cols],
                                      "fields": [c["field"] for c in g.cols if c["field"]],
                                      "rows": len(g.rows), "grid": True}
            if not index["meta"]:
                index["meta"] = head_meta(html)
        else:
            note = "LOGIN-REDIRECT" if has_login else ("no-grid" if status == 200 else "http")
            index["reports"][slug] = {"label": label, "http": status, "grid": False, "note": note}
        time.sleep(DELAY)

    # 2) Страницы модели тенанта
    for name, path in PAGES.items():
        status, html = fetch(path)
        print("[page] %-20s HTTP %s  %d B" % (name, status, len(html)))
        with open(os.path.join(OUT, "raw", "page_%s.html" % name), "w") as f:
            f.write(html)
        index["pages"][name] = {"http": status, "bytes": len(html)}
        time.sleep(DELAY)

    # 3) Разведка JSON-API из фронтового бандла
    status, js = fetch("/frontend/yii-assets/report/index.js")
    if status == 200:
        urls  = sorted(set(re.findall(r'report/json[\w./-]*', js)))
        types = sorted(set(re.findall(r'type["\']?\s*[:=]\s*["\']([\w-]+)', js)))
        index["json_api"] = {"urls": urls, "types": types}
        print("\nJSON-API из бандла: %d url, %d type" % (len(urls), len(types)))
        for u in urls:
            print("   ", u)

    with open(os.path.join(OUT, "_index.json"), "w") as f:
        json.dump(index, f, ensure_ascii=False, indent=2)

    # 4) Человекочитаемый обзор структуры: кто что отдаёт + пример строки
    md = ["# Структура отчётов Dooglys (автообзор)", "",
          "Период: `%s`  |  Тенант: `%s`" % (PERIOD, index["meta"].get("tenant-id", "?")),
          "", "| Отчёт | URL | Колонки | Строк |", "|---|---|---|---|"]
    for slug, r in index["reports"].items():
        if r.get("grid"):
            md.append("| %s | `/report/%s` | %d | %d |" %
                      (r["label"], slug, len(r["cols"]), r["rows"]))
        else:
            md.append("| %s | `/report/%s` | — | ⚠️ %s |" %
                      (r["label"], slug, r.get("note", "—")))
    md.append("\n---\n")
    for slug, r in index["reports"].items():
        if not r.get("grid"):
            continue
        md.append("## %s — `/report/%s`" % (r["label"], slug))
        md.append("Поля (machine): `%s`" % ", ".join(r["fields"]) if r["fields"] else "")
        md.append("\n| # | Колонка | Поле |\n|---|---|---|")
        g = json.load(open(os.path.join(OUT, "%s.grid.json" % slug)))
        for n, c in enumerate(g["columns"]):
            md.append("| %d | %s | %s |" % (n, c["label"], c["field"] or ""))
        if g["rows"]:
            md.append("\nПример строки: `%s`" % json.dumps(g["rows"][0]["cells"], ensure_ascii=False))
        if g["total"]:
            md.append("Итого: `%s`" % json.dumps(g["total"], ensure_ascii=False))
        md.append("")
    with open(os.path.join(OUT, "_structure.md"), "w") as f:
        f.write("\n".join(md))

    print("\nГотово → %s/" % OUT)
    print("  _structure.md — обзор «кто что отдаёт» (читай первым)")
    print("  _index.json   — сводка + JSON-API")
    print("  <slug>.grid.json / raw/<slug>.html — фикстуры")


if __name__ == "__main__":
    main()
