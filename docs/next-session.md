# Следующая сессия разработки — актуально на 2026-06-26

> Заменяет: `archive/next-session-kickoff-2026-06-25.md`, `archive/demo-polish-handoff-2026-06-22.md`,
> `archive/eval-stage2-handoff-2026-06-21.md`. Те закрыты — смотреть только как историю.

---

## Где мы сейчас

**Закрыто:**
- Итерация «шва» (задачи 1–7 + находка A) — детали в `docs/05-quality-and-security-roadmap.md`.
- Трек C частично: C1 (Report-API клиент) + C2+C3 (каталог + фикстуры personnel) + C4 (роутинг запросов personnel) — всё на фикстурах `report_personnel`.
- Прогноз Q2 (run-rate), каналы оплаты, follow-up диалог, безопасность 62/62, Telegram-транспорт, Excel, живой API Dooglys (payment + products).

**Текущий eval:** `571/615` planners pass (promppts corpus, commit `f705bb7`), pipeline 18/18 stub → см. `bench/LEDGER.md`.

---

## Следующие шаги

### Трек C — Q4 персонал/кухня (в работе)

- [ ] **C5.** Флип на live по доступу B1 + **golden-контракты** на границе Report-API
      (сырой ответ → нормализованные строки). Блокер: `x-context` от тех-стороны Dooglys.
- [ ] **C6.** Тесты + Тир 0 зелёный (`make vet && make test`); если правился промпт — `eval-host` (пользователь).

### Трек B — доступ к Report-API (действие пользователя, параллельно)

- [ ] **B1.** Запросить у тех-стороны Dooglys `x-context` (tenant_id + tenant_domain) / доступ к
      `report.production.dgs`. НЕ через Дмитрия (продажи) — через тех-сторону.

### После C + деплой

Следующий крупный рубеж — деплой на серверы заказчика. Репозиторий сейчас приводится в порядок
под эту задачу (ветка `chore/repo-cleanup`). Документ для деплоя: `docs/DEPLOYMENT.md` (создаётся).

---

## Рабочие правила

- **Риг-прогоны запускаю я (пользователь), не ассистент.** Ассистент готовит Тир 0 (`make test`/`vet`) и отдаёт команду; реальный LLM (`make eval-host`/`pipeval-host`) гоняю я.
- **Любая правка промпта планировщика → полный `eval-host` до коммита** (узкий фикс ронял 523→507).
- **Прогоны сохранять по контракту:** `… 2>&1 | tee bench/runs/$(date +%Y-%m-%d_%H%M)_<гарнес>_<корпус>_<режим>_$(git rev-parse --short HEAD).log`, затем дописать строку в `bench/LEDGER.md`.
- **Git-коммиты на английском, без Co-Authored-By.** Коммитить — только когда прошу.
- **Стенд:** деплой `docker compose up --build` запускаю я; бэкенд `bot` на `:8088`, веб на `:8090`, снаружи — `bot.bubnov.site`.

---

## Карта проекта (куда смотреть)

| Область | Файлы |
|---|---|
| Движок расчётов | `internal/engine/` — Plain/TopN/Compare/Contribution/ChannelShare/Forecast |
| Планировщик | `internal/planner/llm.go` + `stub.go` + `refine.go`; `internal/plan/` — схема/валидатор |
| Данные | `internal/dooglys/` — `apiclient.go` (API v1), `reportclient.go` (Report-API), `fixture.go`, `composite.go` |
| Нарратор / Советник | `internal/narrator/`, `internal/advisor/` (⏸️ заморожен) |
| HTTP / Telegram | `cmd/server/`, `cmd/bot/`; `internal/transport/` |
| Бенчмарки | `bench/LEDGER.md` (тренд), `test/eval/*.jsonl` (корпуса), `bench/summarize.sh` |
| Доки / видение | `docs/11-mvp-and-roadmap.md` (якорь), `docs/00-pilot-vision.md`, `docs/01-architecture.md` |
| Спека Report-API | `docs/report.yml`; фикстуры: `docs/contracts/fixtures/entities/report_*.json` |
| Роудмап качества | `docs/05-quality-and-security-roadmap.md` |
| Деплой | `docs/DEPLOYMENT.md` (создаётся), `docker-compose.yml`, `Makefile` |
