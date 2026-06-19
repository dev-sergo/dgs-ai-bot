#!/usr/bin/env python3
"""
Безопасный краулер справочников/сущностей Dooglys.
Сидится из навбара, обходит ТОЛЬКО read-only GET-индексы, сохраняет сырой HTML
и извлекает GridView (поля+значения) в docs/contracts/fixtures/entities/.

Безопасность: пропускает любые ссылки с data-method=post и опасными сегментами
(delete/update/create/logout/service/import/export/transfer/repost/relations).

Запуск:
    export DGS_COOKIE='advanced-backend=...; _csrf-backend=...; _identity-backend=...'
    python3 tools/crawl_entities.py
"""
import os, sys, re, json, time
from urllib.parse import urljoin, urlparse
from html.parser import HTMLParser

COOKIE = os.environ.get("DGS_COOKIE", "")
if not COOKIE:
    sys.exit("ОШИБКА: задай DGS_COOKIE")
os.environ["DGS_COOKIE"] = COOKIE  # для импорта харвестера

import importlib.util
_spec = importlib.util.spec_from_file_location("h", os.path.join(os.path.dirname(__file__), "harvest_contracts.py"))
h = importlib.util.module_from_spec(_spec); _spec.loader.exec_module(h)

BASE   = os.environ.get("DGS_BASE", "https://google.dooglys.com")
HOST   = urlparse(BASE).netloc
OUT    = "docs/contracts/fixtures/entities"
MAXPG  = int(os.environ.get("DGS_MAXPG", "120"))
DELAY  = float(os.environ.get("DGS_DELAY", "0.4"))
SEED   = os.environ.get("DGS_SEED", "/report/main")

# Опасные подстроки в href — НИКОГДА не ходим
BAD = ("delete", "/update/", "/update?", "/create", "logout", "service/menu",
       "/import", "/export", "transfer", "repost", "get-relations",
       "has-relations", "send-feedback", "login=", "/robot", "/retail",
       "/call-center", "receipt-editor", "?sort=", "page=", "logical")


class Links(HTMLParser):
    """Собирает <a href> вместе с data-method, чтобы отсеять POST-экшены."""
    def __init__(self):
        super().__init__(); self.out = []
    def handle_starttag(self, t, a):
        if t != "a":
            return
        ad = dict(a)
        href = ad.get("href")
        if href:
            self.out.append((href, (ad.get("data-method") or "").upper()))


def safe(href, method):
    if method == "POST":
        return False
    if href.startswith("#") or href.startswith("javascript"):
        return False
    low = href.lower()
    if any(b in low for b in BAD):
        return False
    u = urljoin(BASE + "/", href)
    p = urlparse(u)
    if p.netloc and p.netloc != HOST:
        return False
    return True


def path_key(href):
    u = urljoin(BASE + "/", href)
    return urlparse(u).path.rstrip("/")


def main():
    os.makedirs(OUT, exist_ok=True)
    seen, queue = set(), [SEED]
    seen.add(path_key(SEED))
    visited, skipped, external = [], set(), set()
    n = 0

    while queue and n < MAXPG:
        href = queue.pop(0)
        path = path_key(href)
        status, html = h.fetch(href)
        n += 1
        has_login = "site/login" in html or "LoginForm" in html
        has_grid = "grid-view" in html or "table-wrap" in html
        fn = (path.strip("/").replace("/", "_") or "root")
        with open(f"{OUT}/{fn}.html", "w") as f:
            f.write(html)
        rec = {"url": path, "http": status, "grid": False}
        if status == 200 and has_grid and not has_login:
            g = h.Grid(); g.feed(h.grid_chunk(html))
            if g.cols:
                json.dump({"url": path, "columns": g.cols, "rows": g.rows,
                           "total": g.foot, "row_count": len(g.rows)},
                          open(f"{OUT}/{fn}.grid.json", "w"), ensure_ascii=False, indent=2)
                rec.update(grid=True, cols=[c["label"] for c in g.cols],
                           fields=[c["field"] for c in g.cols if c["field"]],
                           rows=len(g.rows))
        elif has_login:
            sys.exit("Cookie протух (login-redirect). Возьми свежий и перезапусти.")
        visited.append(rec)
        print("[%3d] %-45s HTTP %s %s" %
              (n, path, status, "grid:%d" % rec.get("rows", 0) if rec["grid"] else "—"))

        # новые ссылки
        L = Links(); L.feed(html)
        for hf, m in L.out:
            u = urljoin(BASE + "/", hf)
            if urlparse(u).netloc not in ("", HOST):
                external.add(u); continue
            if not safe(hf, m):
                skipped.add(path_key(hf)); continue
            k = path_key(hf)
            if k and k not in seen:
                seen.add(k); queue.append(hf)
        time.sleep(DELAY)

    grids = [v for v in visited if v["grid"]]
    json.dump({"visited": visited, "skipped_dangerous": sorted(skipped),
               "external": sorted(external)},
              open(f"{OUT}/_crawl.json", "w"), ensure_ascii=False, indent=2)
    print("\nПосещено: %d | с сеткой: %d | пропущено опасных: %d | внешних: %d"
          % (len(visited), len(grids), len(skipped), len(external)))
    print("Сетки:", ", ".join(sorted(v["url"] for v in grids)))
    print("→ %s/  (_crawl.json — карта всех URL)" % OUT)


if __name__ == "__main__":
    main()
