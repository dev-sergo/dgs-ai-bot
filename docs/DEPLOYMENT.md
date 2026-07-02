# Деплой — Dooglys AI-bot (Telegram-only, N ботов)

Демо-канал — **Telegram**. Один процесс (`cmd/bot`) поднимает **по боту на каждого
тенанта** из `TENANTS`; каждый бот жёстко привязан к своему тенанту и whitelist'у,
общий движок резолвит источник данных по `tenant_id` (изоляция). Деплой — вручную
через `docker compose` (CI/CD намеренно не используем; всё просто и прозрачно).

Портов наружу нет: боты ходят по ИСХОДЯЩИМ к `api.telegram.org`, роутеру моделей
(`LLM_BASE_URL`) и `api.dooglys.com`.

---

## Быстрый старт

```bash
cp .env.example .env        # заполнить: bot-токены, LLM_API_KEY, access-token'ы, allowlist'ы
docker compose up -d --build

docker compose ps           # состояние
docker compose logs -f      # логи (старт каждого бота, ошибки)
```

Остановить: `docker compose down`. Обновить после `git pull`:

```bash
git pull
docker compose up -d --build --force-recreate
```

**Предпосылки на сервере:** установлены `git`, `docker`, `docker compose`; исходящий
доступ к `github.com` (для `git pull`), `litellm.site.avtosushi.net` (роутер) и
`api.dooglys.com`. Проверить достижимость до первого запуска:

```bash
curl -sSf https://api.dooglys.com/api/v1/reports >/dev/null && echo dooglys-ok
curl -sS  https://litellm.site.avtosushi.net/v1/models -H "Authorization: Bearer $LLM_API_KEY" | head
```

---

## Переменные окружения (`.env`)

Полный заполняемый шаблон — [`.env.example`](../.env.example). Ниже — что за что отвечает.

### Приложение

| Переменная | Значение (демо) | Описание |
|---|---|---|
| `APP_ENV` | `prod` | `prod` → строгие инварианты: пустой allowlist у тенанта = fail-fast на старте. `dev` → послабления. |
| `PLANNER_MODE` | `llm` | `llm` — реальная модель через роутер; `stub` — детерминированные планы без сети/GPU. |

### LLM-роутер

| Переменная | Значение | Описание |
|---|---|---|
| `LLM_BASE_URL` | `https://litellm.site.avtosushi.net` | Базовый домен OpenAI-совместимого роутера **без** `/v1` (клиент добавляет `/v1/chat/completions`). |
| `LLM_MODEL` | `gemma-4-31b` | Идентификатор модели на роутере. |
| `LLM_API_KEY` | 🔒 | Bearer-токен роутера. |
| `LLM_FORCE_JSON` | `true` | `response_format=json_object`. Если роутер/модель не поддержит → `false` (parse+repair). Проверить curl'ом. |
| `LLM_TIMEOUT` | `180s` | Таймаут одного запроса к модели. |

### Источник данных (Report-API, внешний контур)

| Переменная | Значение | Описание |
|---|---|---|
| `DGS_CLIENT` | `api` | `api` — живой Report-API; `fixture` — локальные JSON (оффлайн-демо). |
| `DGS_REPORT_AUTH` | `token` | Режим авторизации: `token` (внешний `api.dooglys.com`) / `xcontext` (внутренний, кубы). |
| `DGS_REPORT_BASE` | `https://api.dooglys.com/api/v1/reports` | База Report-API (внешний путь с префиксом `/reports`). |
| `DGS_ACCESS_TOKEN` | 🔒 (опц.) | Общий `access-token`, если один на всех. Разные → задать пер-тенантно. |

> `DGS_LOGIN`/`DGS_PASSWORD` для 6 отчётов ТЗ **не нужны** (payment/products идут через
> Report-API). Без них отключается лишь «живой индекс товаров» — распознавание названий
> товаров берётся из фикстур; сами суммы отчётов живые.

### Логи (JSONL на томе `./data`)

| Переменная | Значение | Описание |
|---|---|---|
| `QUERY_LOG_PATH` | `/app/data/queries.jsonl` | Датасет вопрос→план→ответ (tenant/user в каждой строке). Пусто → выкл. |
| `FEEDBACK_LOG_PATH` | `/app/data/feedback.jsonl` | Оценки 👍/👎. Пусто → выкл. |

Оба файла append-only, переживают рестарт. Один процесс на все боты → один файл каждого
вида; разделение по тенантам — по полю в строке.

---

## Тенанты (= боты): как завести несколько

1. **Ключи** тенантов — через запятую в `TENANTS` (ключ = произвольный ярлык, удобно взять
   = домену). Каждый ключ = отдельный бот.
2. На каждый ключ `<k>` — блок `TENANT_<k>_*`.

| Переменная | Обяз.? | Описание |
|---|---|---|
| `TENANT_<k>_BOT_TOKEN` | **да** | 🔒 токен @BotFather. Нет токена → fail-fast. |
| `TENANT_<k>_ALLOWLIST` | **да (prod)** | Смешанный whitelist (см. ниже). Пустой в `prod` → fail-fast. |
| `TENANT_<k>_ID` | опц. | `tenant_id` (default = ключ). Должен совпадать с записью в `tenants.example.json`. |
| `TENANT_<k>_DOMAIN` | опц. | `tenant-domain` Report-API (default = ID). |
| `TENANT_<k>_ACCESS_TOKEN` | опц. | 🔒 свой `access-token`; пусто → общий `DGS_ACCESS_TOKEN`. |

Пишешь только то, что реально отличается (ID/DOMAIN дефолтятся от ключа, токен может быть
общим). **Добавить бота = дописать ключ в `TENANTS` + блок `TENANT_<k>_*`; код не меняется.**

```env
TENANTS=rukagreka,tenant2,tenant3,tenant4
TENANT_rukagreka_BOT_TOKEN=111:AAA
TENANT_rukagreka_ALLOWLIST=@owner_ivan, 100200300
TENANT_tenant2_BOT_TOKEN=222:BBB
TENANT_tenant2_DOMAIN=second-domain
TENANT_tenant2_ALLOWLIST=@owner_maria
# … tenant3, tenant4 аналогично
```

### Allowlist: `@username` и/или `chat_id`

`TENANT_<k>_ALLOWLIST` — csv, каждый элемент распознаётся сам:

- элемент из **одних цифр** → числовой `chat_id` (неизменяемый, надёжный). Узнать id: написать `@userinfobot`.
- **всё остальное** → `@username` (регистр и `@` не важны: `@Ivan` == `ivan`). Удобно, но
  username изменяем — если человек сменит `@handle`, доступ отвалится. Для железобетонного
  доступа конкретного человека — числовой id.

Можно смешивать: `@owner_petr, 700800900`. В `APP_ENV=prod` хотя бы один элемент обязателен.

---

## Безопасность боевого контура

| Слой | Защита | Где |
|---|---|---|
| **Telegram whitelist** | Доступ по `chat_id` и/или `@username`; в `APP_ENV=prod` пустой allowlist = fail-fast на старте. Чужой отбит на каждом боте. | `config.ValidateTelegram`, `telegram.Bot.allowed` |
| **Анти-спам** | Per-chat rate-limit (10 запросов/мин) поверх капа в 8 одновременных `Ask`. Превышение → мягкий ответ, не тихий дроп. | `internal/transport/telegram` |
| **Изоляция тенантов** | Бот жёстко на своём `tenant_id` (не из ввода); сессии скоупятся `tg:<tenant>:<chatID>`; у каждого тенанта свой client+resolver. | `bootstrap`, `telegram.Bot` |
| **Секреты вне LLM/логов** | `plan.go` без tenant_id; `config.Summary()` печатает секреты как `set/unset`. | `internal/config`, `internal/planner` |
| **Graceful-деградация** | Сбой `client.Fetch()` → человеческий ответ «источник временно недоступен», реальная ошибка — в лог. | `app.dataUnavailable` |

> HTTP-транспорт (`cmd/server`) в этом деплое не поднимается (Telegram-only). Если позже
> публикуешь его наружу — ставь rate-limit на обратном прокси перед `/ask`/`/export`.

---

## Права на том логов

`./data` монтируется в `/app/data`, куда пишет процесс под nonroot (uid 65532). Если в
логах видишь предупреждение, что датасет не открылся (нет прав на смонтированный каталог) —
бот продолжит работать без лога. Чтобы включить запись, выдай права каталогу на хосте:

```bash
mkdir -p ./data && sudo chown -R 65532:65532 ./data   # либо chmod 777 ./data
```

---

## Проверка перед демо (smoke)

1. `docker compose logs` — каждый бот стартовал (строка `telegram bot started` с username и
   размером allowlist на тенанта).
2. Написать каждому боту простой вопрос из scope (напр. «выручка за вчера») → пришёл ответ.
3. **Изоляция:** разрешённый на боте A `chat_id`/`@username` не имеет доступа к боту B; вопрос
   в бот A возвращает числа тенанта A, не B.
4. **Модель:** перед демо прогнать eval планировщика на боевой `gemma-4-31b` (роутер
   публичный, гоняется с хоста):
   ```bash
   LLM_API_KEY=<bearer> LLM_BASE_URL=https://litellm.site.avtosushi.net \
     LLM_MODEL=gemma-4-31b make eval-smoke   # быстрый сигнал (~5 мин); затем make eval-host
   ```

---

## Источник данных: режимы `DGS_CLIENT`

- **`api`** (демо) — 6 отчётов ТЗ через Report-API (`payment`, `source-order`, `products`,
  `categories`, `personnel`, `cash-on-hand`, `cash-income-outcome`); остальное — фикстуры.
- **`fixture`** (CI/оффлайн) — все отчёты из локальных JSON (`$FIXTURES_PATH/*.grid.json`),
  сеть к Dooglys не нужна.

Конфиг тенантов (таймзона/валюта/точки) — `docs/contracts/fixtures/tenants.example.json`,
ключуется по `tenant_id`. Для боевых тенантов значения `TENANT_<k>_ID` должны совпадать с
ключами в этом файле (иначе тенант получит дефолты).
