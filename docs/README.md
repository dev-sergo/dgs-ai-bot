# Dooglys AI-bot — документация

Аналитический AI-ассистент над POS-платформой [Dooglys](https://dooglys.com): принимает
произвольный текстовый запрос владельца заведения и возвращает аналитику по его данным.

> **Якорный документ** (MVP, демо, дорожная карта): **[11-mvp-and-roadmap.md](11-mvp-and-roadmap.md)**
>
> **Тренд качества** (история прогонов бенчмарков): **[../bench/LEDGER.md](../bench/LEDGER.md)**
>
> **Статус качества и журнал итераций** (где мы сейчас): [05-quality-and-security-roadmap.md](05-quality-and-security-roadmap.md)

---

## Для нового участника — с чего читать

1. [11-mvp-and-roadmap.md](11-mvp-and-roadmap.md) — что строим, что работает сейчас, граница MVP, дорожная карта
2. [01-architecture.md](01-architecture.md) — архитектура, путь запроса, sandwich-безопасность
3. [next-session.md](next-session.md) — текущий фокус разработки + рабочие правила

Для деплоя: [DEPLOYMENT.md](DEPLOYMENT.md) *(создаётся)*.

---

## Состав документации

### Стратегия и продукт

| Документ | О чём |
|---|---|
| [11-mvp-and-roadmap.md](11-mvp-and-roadmap.md) | **Якорь.** Что строим, что есть сейчас, граница MVP, дорожная карта фаз, открытые вопросы |
| [00-pilot-vision.md](00-pilot-vision.md) | Продуктовое видение пилота: идеология, демо-сценарий, план месяц1/2/3 |
| [07-customer-overview.md](07-customer-overview.md) | Что умеет бот — обзор для заказчика |

### Архитектура и разработка

| Документ | О чём |
|---|---|
| [01-architecture.md](01-architecture.md) | Архитектура: слои, путь запроса, планировщик, sandwich-безопасность |
| [02-data-contracts-and-open-questions.md](02-data-contracts-and-open-questions.md) | Контракты Dooglys API: три режима (fixture / JSON API v1 / Report-API) |
| [03-testing-strategy.md](03-testing-strategy.md) | Стратегия тестирования: пирамида, корпуса, eval без GPU |
| [04-development-plan.md](04-development-plan.md) | White-list, AnalysisPlan, дерево приложения, майлстоуны |
| [08-telegram-transport.md](08-telegram-transport.md) | Telegram-транспорт: развязка с ядром, рендер, рабочие решения |
| [glossary.md](glossary.md) | Глоссарий: этапы запроса, архитектурные термины, концепты LLM/тестирования |

### Качество и операции

| Документ | О чём |
|---|---|
| [05-quality-and-security-roadmap.md](05-quality-and-security-roadmap.md) | Живой журнал итераций качества и безопасности |
| [next-session.md](next-session.md) | Текущий статус, следующие шаги, рабочие правила |
| [DEPLOYMENT.md](DEPLOYMENT.md) *(создаётся)* | Деплой: env-переменные, docker-compose, LLM, режимы клиента |
| [API.md](API.md) *(создаётся)* | HTTP API: `/ask`, `/export`, `/feedback`, `/healthz`, auth, envelope |
| [BENCHMARKS.md](BENCHMARKS.md) *(создаётся)* | Реестр корпусов eval/bench, схемы Expect, команды запуска |

### Дизайн-документы (не реализовано)

| Документ | О чём |
|---|---|
| [design/quality-judge.md](design/quality-judge.md) | LLM-судья Tier 0: рубрика, калибровка, угрозы — **дизайн, кода нет** |
| [design/feedback-signal.md](design/feedback-signal.md) | 👍/👎 петля обратной связи: логирование, UI-кнопки — **дизайн, кода нет** |
| [06-assistant-design.md](06-assistant-design.md) | Консультант (advisor): insight-bundle, intent=advice, eval — **реализован, заморожен как точка входа MVP** |

### Референсы

| Документ | О чём |
|---|---|
| [report.yml](report.yml) | Спека Report-API Dooglys (server-side отчёты: payment/personnel/kitchen/abc/…) |
| [contracts/fixtures/_structure.md](contracts/fixtures/_structure.md) | Схема нормализованных данных Dooglys (~50 отчётов, PII redacted) |
| [reference/](reference/) | Бинарные референсы: ТЗ Пети, дашборды, обзор `.docx` |
| [research/restik-competitive.md](research/restik-competitive.md) | Конкурентный анализ Restik AI |
| [archive/](archive/) | Устаревшие handoff-доки (для истории) |

---

## Ключевые решения (одной строкой)

- **LLM не считает числа.** LLM = планировщик + нарратор. Вся математика — детерминированный Go.
- **Sandwich-безопасность.** LLM зажат между валидируемыми слоями; `tenant_id` и токены — только server-side.
- **Конструктор из white-list.** Произвольный запрос → структурированный план из разрешённого словаря.
- **Report-API как источник правды.** Server-side отчёты Dooglys — наш движок считает только производные (прогноз/сравнения/синтез).
- **Локальные модели.** vLLM + Qwen2.5; облако не используется.
