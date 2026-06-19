# 04. План разработки (основной вектор)

Итоговый план: что строим, дерево приложения, фикстуры, тесты, и как масштабируем.
Опирается на нормализованные данные в [contracts/fixtures](contracts/fixtures/) и принципы из [01-architecture.md](01-architecture.md).

---

## 1. Scope MVP и white-list отчётов

**Граница (подтверждена):** единственный интерфейс данных — отчётные endpoints Dooglys
с фильтрами `BaseReportForm[*]`. Только чтение. Никаких операций записи в MVP.

**White-list MVP — 4 отчёта** (по которым есть фильтры от заказчика):

| slug | Отчёт | Тип | Назначение |
|---|---|---|---|
| `payment` | Выручка | timeseries по дням | Class A + база Class B |
| `products` | Товары | агрегат по товару | Class A + «что просело» |
| `paycheck` | Чеки | построчный | lookup по чекам |
| `orders` | Заказы | построчный | lookup по заказам |

Остальные отчёты (`categories`, `expected-profit`, `abc`, …) уже описаны в
[catalog.example.yaml](contracts/fixtures/catalog.example.yaml) — включаются **флагом в каталоге**,
без изменения кода. Это и есть механизм роста white-list.

---

## 2. Модель данных и каталог

- **Каноническая строка отчёта** = нормализованный `*.json` (`{report, label, rows:[{field:value}]}`).
  `DooglysClient` (и фикстурный, и реальный) возвращают этот формат.
- **Каталог** (`configs/catalog.yaml`, из примера в fixtures) — единый источник правды:
  отчёт → поля (`key`, `label`, `unit`, `pii`), маппинг на endpoint, поддерживаемые фильтры.
  Из него генерируются: словарь для LLM, парсер колонок, проекция PII.
- **Единицы:** RUB — рубли, 2 знака; `count` — целое; `date` — ISO; profit = «ожидаемая»
  (прогноз, бывает отрицательной).

### White-list фильтров (из реальных URL)

Общие для всех: `period` (обяз.), `locality_id[]`, `sale_point_id[]`.
По отчётам добавляются: `user_id[]`, `product_id[]`, `product_category_id[]`,
`payment_type[]`(card/cash/online/sbp), `processing_status[]`, `order_order_type[]`,
`source[]`, `special_id[]`, `customer_id[]`, `order_number`, `phone_number`,
`cost_from`/`cost_to`, `return_reason[]`, `include_zero_price`.

uuid-фильтры: модель выдаёт **имя**, детерминированный `resolver` мэтчит имя→uuid по
справочникам. enum-фильтры — фиксированные значения из каталога.

---

## 3. AnalysisPlan (контракт LLM → движок)

```jsonc
{
  "version": "1",
  "class": "A | B",                       // прямой отчёт | аналитика
  "report": "payment",                    // slug из white-list
  "metrics": ["revenue", "avg_check"],    // канонические поля (НЕ pii)
  "group_by": ["date"],                   // измерения
  "period": { "kind": "relative", "token": "last_7_days" },  // или {kind:"explicit", from,to}
  "compare_to": { "token": "prev_period" },                  // только class B
  "method": "plain | compare | contribution | top_n",
  "top_n": 10,
  "filters": [                            // имена, не uuid
    { "field": "sale_point", "op": "in", "values": ["Выкса"] },
    { "field": "payment_type", "op": "in", "values": ["card"] }
  ],
  "output": { "format": "auto|text|xlsx" },
  "confidence": 0.0
}
```

Валидатор: схема + white-list (report/metrics/filters/enum-значения), запрет `pii`-полей,
проставление `tenant_id` server-side, проверка обязательного `period`. Невалидно/мало
данных → уточняющий вопрос или честный отказ.

### Движок (детерминированный Go)
- `plain` — отдать отчёт с фильтрами;
- `compare` — два периода + дельта (abs/pct);
- `contribution` — вклад товара/категории в изменение метрики («почему просела выручка»);
- `top_n` — топ/анти-топ.
Все числа считаются здесь. Резолв `period` токенов → абсолютные даты **по таймзоне точки/тенанта**.

---

## 4. Безопасность и PII (усилено каталогом)

1. `tenant_id`/токены — только server-side, никогда от LLM.
2. LLM-выход ограничен схемой плана; запрос `pii`-полей отклоняется валидатором.
3. **PII-проекция:** поля с `pii:true` не попадают ни в контекст нарратора, ни в ответ
   (маскируются/исключаются). Для построчных `orders`/`paycheck` в MVP отдаём агрегаты/
   обезличенные колонки.
4. Изоляция тенанта: каждый запрос данных скоупится по `tenant_id`.
5. Числа рендерятся из результата движка (плейсхолдеры), модель их не пишет.

---

## 5. Дерево приложения

```
dgs-ai-bot/
├── cmd/server/main.go              # точка входа, wiring, конфиг
├── configs/
│   ├── catalog.yaml                # декларативный каталог (из fixtures)
│   └── config.yaml                 # порт, LLM endpoint/model, источник данных
├── internal/
│   ├── transport/http/             # POST /ask, GET /healthz; request_id, rate-limit
│   ├── tenantctx/                  # граница авторизации; MVP: стаб (tenant_id из заголовка)
│   ├── catalog/                    # загрузка/валидация каталога, генерация словаря для LLM
│   ├── llm/                        # OpenAI-совместимый клиент llama.cpp (+ json-schema/grammar)
│   ├── planner/                    # текст → AnalysisPlan | StubPlanner (тесты без GPU)
│   ├── plan/                       # типы AnalysisPlan + валидатор (white-list + PII)
│   ├── resolver/                   # имя → uuid по справочникам
│   ├── dates/                      # относительный токен → даты по таймзоне
│   ├── dooglys/                    # интерфейс Client | FixtureClient | ScrapeClient(позже)
│   ├── parse/                      # GridView → нормализованная строка (RU числа/даты)
│   ├── engine/                     # plain/compare/contribution/top_n
│   ├── envelope/                   # единый результат
│   ├── render/                     # text | xlsx (excelize)
│   └── narrator/                   # результат → текст | StubNarrator
├── test/
│   ├── integration/                # сквозные сценарии (stub LLM + fixtures)
│   └── eval/                       # prompts.jsonl + раннер качества/латентности
├── scripts/ask.sh                  # cURL-обёртка: ./ask.sh "выручка за неделю"
├── docker-compose.yml              # сервис (LLM — внешний, на риге пользователя)
├── Makefile                        # build/test/bench/run
└── docs/                           # эта документация
```

`FixtureClient` = in-memory движок над нормализованными `*.json`: применяет
`BaseReportForm`-фильтры (период/точка/…) к строкам — эмулирует реальный endpoint.
Поэтому новых файлов фикстур почти не нужно.

---

## 6. Майлстоуны (каждый — рабочий инкремент)

| M | Содержание | Критерий готовности |
|---|---|---|
| **M0** | Каркас, конфиг, `llm`-клиент на риг, `/healthz`, `/ask` (эхо плана) | cURL → валидный `AnalysisPlan` от реальной модели |
| **M1** | Class A на фикстурах: 4 отчёта, план→валидатор→FixtureClient→engine(plain)→render(text). StubPlanner/StubNarrator | `./ask.sh "выручка за период X"` отдаёт таблицу; интеграционные тесты зелёные без GPU |
| **M2** | Фильтры + `resolver`(имя→uuid) + `dates`(токены/таймзоны) + PII-проекция | Запросы с точкой/сотрудником/типом оплаты; даты по tz; PII не утекает |
| **M3** | Class B: `compare` + `contribution` + нарратив (2-й вызов LLM) | «почему упала выручка за месяц» даёт объяснение с вкладами; числа из движка |
| **M4** | Реальный источник: `ScrapeClient` (или внешний API, если найдём) за интерфейсом, под флагом | Переключение fixtures↔live без изменения ядра |
| **M5** | xlsx-экспорт, eval/бенчмарк, семантический кэш (nomic/bge) | `make bench` даёт точность+латентность; «выгрузи в excel» работает |

---

## 7. Фикстуры, которые создаём

В основном переиспользуем нормализованные данные; добавляем тестовую обвязку:
- **Канон отчётов** — существующие `contracts/fixtures/*.json` (копия в `test/fixtures` или чтение напрямую).
- **Справочники для resolver** — извлечь из `entities/structure_sale-point|locality|user`,
  `nomenclature_product` → таблицы имя→uuid (3 мок-тенанта).
- **Мок-тенанты** — [tenants.example.json](contracts/fixtures/tenants.example.json) (готов).
- **Канонические планы (stub LLM)** — `test/fixtures/plans/*.json`: для каждого тестового
  запроса заранее заданный `AnalysisPlan` (детерминизм без GPU).
- **Golden-набор** — `test/eval/prompts.jsonl`: `(запрос → ожидаемые свойства плана)`.
- **Пары grid↔normalized** — для теста парсера (вход `*.grid.json`, ожидание `*.json`).

---

## 8. Тестирование (закладываем сразу)

| Тест | Уровень | GPU | Что проверяет |
|---|---|---|---|
| `parse_test` | unit | нет | GridView → нормализ. строка (все отчёты), RU-числа/даты |
| `dates_test` | unit | нет | токены `today/last_7_days/...` → даты для Москвы и Екатеринбурга |
| `validator_test` | unit | нет | white-list accept/reject, override `tenant_id`, запрет PII |
| `resolver_test` | unit | нет | имя→uuid, фаззи-матч, неоднозначность → уточнение |
| `engine_test` | unit | нет | plain/compare/contribution/top_n на фикстурах с предрасчётом |
| `fixtureclient_test` | unit | нет | фильтрация по периоду/точке возвращает ожидаемый срез |
| `pii_test` | unit | нет | pii-поля не в контексте нарратора и не в ответе |
| `integration_test` | integ | нет (stub) | сквозь: выручка/топ-товары/«почему упала»/уточнение/xlsx |
| `isolation_test` | integ | нет | тенант A не получает данные B |
| `eval/bench` | eval | да | точность плана + латентность + токены на golden-наборе (`make bench`) |

Тестовые контуры:
- **CI/быстрый** — всё, кроме eval: StubPlanner + FixtureClient, детерминированно, без GPU.
- **Бенчмарк качества** — раннер бьёт по реальной модели на риге, пишет таблицу
  «запрос → план → корректность → латентность». Это твой cURL-бенч.
- **Ручной** — `scripts/ask.sh "текст"` против поднятого сервиса.

---

## 9. Масштабирование (вектор развития)

- **Новые отчёты** — флаг в каталоге (схемы 33 отчётов уже есть). Ноль кода ядра.
- **Новые каналы** (Telegram/VK/моб./админка) — адаптеры перед `/ask`, ядро не меняется.
- **Реальный источник** — `ScrapeClient` или внешний API за `DooglysClient`; решается отдельно
  (открытый вопрос: cookie-сессия 1ч vs официальный API по `access-token`).
- **Производительность** — сервис stateless + Redis (кэш ответов по `tenant_id+params_hash`,
  pending-state уточнений). Узкое место — LLM; worker-pool/очередь перед ригом. На 150 тенантов
  / ~2k запросов в день одного рига достаточно.
- **Качество** — eval-harness + трейсинг + feedback 👍/👎; семантический кэш планов (nomic/bge уже на риге).
- **Гибкость** — Tier 2 (sandbox над scoped-данными) для произвольной аналитики (фаза 3).
- **Запись** — фаза 4: структурное изменение → preview → подтверждение → выполнение, роли, аудит.
```
