# Dooglys AI-bot

Аналитический AI-ассистент над POS-платформой [Dooglys](https://dooglys.com).
Владелец или управляющий заведения задаёт вопрос обычными словами — бот возвращает точный ответ по реальным данным.
Числа считает детерминированный Go-движок, не нейросеть.

**Стек:** Go 1.23 · React/Vite · Qwen2.5-32B (llama.cpp/vLLM) · Docker Compose

## Быстрый старт

```bash
cp .env.example .env          # заполнить: DGS_LOGIN, DGS_PASSWORD, LLM_BASE_URL, AUTH_TOKEN
docker compose up --build     # поднимает bot:8088 + web:8090
curl http://localhost:8090/   # веб-интерфейс
curl http://localhost:8088/healthz  # healthcheck
```

Без LLM (детерминированный stub для CI/тестов):
```bash
PLANNER_MODE=stub docker compose up --build
```

## Архитектура (sandwich)

```
запрос → LLM-планировщик → Go-движок (числа) → LLM-нарратор → ответ
              ↑ white-list                 ↑ источник истины
```

LLM управляет только «человеческой» частью — понять вопрос и сформулировать ответ.
Все вычисления (выручка, дельты, прогноз, топ товаров) — детерминированный Go-код.
`tenant_id` и токены Dooglys на сторону LLM не попадают никогда.

## Что умеет сейчас

- Выручка, каналы оплаты, средний чек, возвраты — из живого Dooglys API
- Сравнение периодов, вклад каналов в изменение выручки
- Прогноз выручки на конец периода (run-rate)
- Статистика персонала (через Report-API)
- Топ товаров, фильтр по точке/категории/товару
- Follow-up вопросы (диалоговый контекст)
- Excel-выгрузка, Telegram-бот, веб-интерфейс

## Документация

| | |
|---|---|
| **[docs/11-mvp-and-roadmap.md](docs/11-mvp-and-roadmap.md)** | Что строим, что работает, дорожная карта |
| **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)** | Все env-переменные, режимы, монтирование, production-конфиг |
| [docs/01-architecture.md](docs/01-architecture.md) | Архитектура, sandwich-безопасность, путь запроса |
| [docs/API.md](docs/API.md) | HTTP API: `/ask`, `/export`, `/feedback`, `/healthz` |
| [docs/BENCHMARKS.md](docs/BENCHMARKS.md) | Eval/bench корпуса, команды, тренд качества |
| [bench/LEDGER.md](bench/LEDGER.md) | История прогонов бенчмарков (тренд по коммитам) |
| [docs/README.md](docs/README.md) | Полный индекс документации |

## Разработка

```bash
make test          # unit + integration (stub, без LLM)
make pipeval       # full-pipeline проверка ответов (stub)
make eval-host     # планировщик на реальной LLM (~40 мин)
make pipeval-host  # pipeline на реальной LLM
make build-host    # native macOS binary
```

Прогоны сохранять по контракту `bench/`:
```bash
make eval-host 2>&1 | tee bench/runs/$(date +%Y-%m-%d_%H%M)_eval_prompts_llm_$(git rev-parse --short HEAD).log
```

## Лицензия

Проприетарный код. Все права защищены.
