# Деплой — Dooglys AI-bot

Сервис состоит из трёх контейнеров (собираются через `docker compose up --build`):
- **bot** — HTTP API (`:8088`), принимает запросы `/ask`, `/export`, `/feedback`, `/healthz`.
- **telegram** — Telegram-бот (отдельный процесс над тем же ядром).
- **web** — nginx, раздаёт React-фронт на `:8090` и проксирует `/api/*` → `bot:8088`.

---

## Быстрый старт

```bash
cp .env.example .env        # заполнить секреты (см. ниже)
docker compose up --build   # поднять всё
curl http://localhost:8090/  # проверить UI
curl http://localhost:8088/healthz  # проверить бэкенд
```

---

## Переменные окружения (`.env`)

Все переменные опциональны — у каждой есть дефолт. Секции ниже в порядке приоритета настройки.

### Обязательно для работы с реальными данными

| Переменная | Дефолт | Описание |
|---|---|---|
| `DGS_CLIENT` | `fixture` | Источник данных: `fixture` (локальные JSON) / `api` (JSON API v1) / `http` (HTML-клиент, legacy) |
| `DGS_BASE` | `https://google.dooglys.com` | URL тенанта Dooglys (заменить `google` на реальный домен) |
| `DGS_DOMAIN` | `google` | Tenant-Domain для API v1 (часть URL до `.dooglys.com`) |
| `DGS_LOGIN` | — | Логин для получения токена JSON API v1 (при `DGS_CLIENT=api`) |
| `DGS_PASSWORD` | — | Пароль для получения токена JSON API v1 |
| `DGS_XCONTEXT` | — | JSON `{"tenant_id":"...","tenant_domain":"..."}` для Report-API (personnel/kitchen); пусто → Report-API выключен |
| `DGS_REPORT_BASE` | = `DGS_BASE` | Базовый URL Report-API, если отличается от основного |

### LLM

| Переменная | Дефолт | Описание |
|---|---|---|
| `LLM_BASE_URL` | `http://172.20.10.2:8080` | Эндпоинт OpenAI-совместимого сервера (llama.cpp / Ollama / vLLM) |
| `LLM_MODEL` | `qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07` | Идентификатор модели на LLM-сервере |
| `LLM_API_KEY` | — | Bearer-токен, если LLM-сервер требует auth |
| `LLM_TIMEOUT` | `180s` | Таймаут одного запроса к LLM (`time.ParseDuration` формат) |
| `LLM_FORCE_JSON` | `true` | Включить `response_format=json_object`; выставить `false` если билд не поддерживает |

### Сервис

| Переменная | Дефолт | Описание |
|---|---|---|
| `HTTP_ADDR` | `:8088` | Адрес HTTP-сервера |
| `PLANNER_MODE` | `llm` | `llm` (реальная модель) / `stub` (детерминированный, для CI/демо без LLM) |
| `FIXTURES_PATH` | `docs/contracts/fixtures` | Путь к директории с фикстурами (монтируется в контейнер) |
| `AUTH_TOKEN` | — | Токен демо-гейта (`X-Auth-Token` / `?key=`); пусто → гейт выключен |
| `QUERY_LOG_PATH` | — | Путь к JSONL-файлу лога вопросов/ответов; пусто → не пишем |
| `FEEDBACK_LOG_PATH` | — | Путь к JSONL-файлу оценок 👍/👎; пусто → не пишем |

### Telegram (опционально)

| Переменная | Дефолт | Описание |
|---|---|---|
| `TELEGRAM_TOKEN` | — | Токен бота Telegram; пусто → Telegram-транспорт не запускается |
| `TELEGRAM_ALLOWLIST` | — | CSV list chat_id (например `123456,789012`); пусто → открыт всем |
| `TELEGRAM_TENANT` | `mock_single` | ID тенанта по умолчанию для Telegram-бота |

---

## Монтирование томов

В `docker-compose.yml` два монтирования:

```
./data → /app/data     # JSONL-логи запросов и фидбека (writable)
```

Фикстуры (`docs/contracts/fixtures/`) копируются **в образ** при сборке (`COPY docs/contracts/fixtures /app/fixtures`).
Если хотите обновлять фикстуры без пересборки — добавьте mount:
```yaml
- ./docs/contracts/fixtures:/app/fixtures:ro
```
и выставьте `FIXTURES_PATH=/app/fixtures`.

---

## Режимы источника данных (`DGS_CLIENT`)

### `fixture` (дефолт, CI/тест)
Все отчёты читаются из локальных JSON-файлов `$FIXTURES_PATH/*.grid.json`.
Сеть к Dooglys не нужна. Подходит для тестирования и оффлайн-демо.

### `api` (production)
- Payment + Products: JSON API v1 (`/api/v1/sales/order/list`, token-auth).
- Personnel: Report-API (`/report/personnel`, x-context) — если `DGS_XCONTEXT` задан.
- Остальные отчёты: fallback на фикстуры.

**Минимальный `.env` для production:**
```env
DGS_CLIENT=api
DGS_BASE=https://TENANT.dooglys.com
DGS_DOMAIN=TENANT
DGS_LOGIN=yourlogin
DGS_PASSWORD=yourpassword
LLM_BASE_URL=http://LLM_HOST:8080
LLM_MODEL=your-model-name
AUTH_TOKEN=secret-demo-token
```

### `http` (legacy)
Парсит HTML через cookie-сессию браузера. Требует `DGS_COOKIE`. Не рекомендуется для production.

---

## Мультитенантность

Тенант определяется из запроса (приоритет): заголовок `X-Tenant-ID` → поле `tenant_id` в JSON → дефолт `mock_single`.

Конфиг тенантов (timezone, валюта, точки продаж) — файл `$FIXTURES_PATH/tenants.example.json`.
Для production переименуйте/замените его под реальные данные.

---

## Проверка работоспособности

```bash
# healthcheck (не требует токена)
curl http://localhost:8088/healthz

# тестовый вопрос (требует AUTH_TOKEN если задан)
curl -s -X POST http://localhost:8088/ask \
  -H 'Content-Type: application/json' \
  -H 'X-Auth-Token: YOUR_TOKEN' \
  -d '{"query":"выручка за вчера","tenant_id":"mock_single"}' | jq .

# smoke-тест auth + xlsx
BASE=http://localhost:8088 KEY=YOUR_TOKEN bash scripts/ask.sh
```

---

## Обновление

```bash
git pull
docker compose up --build --force-recreate
```

Логи (`QUERY_LOG_PATH`, `FEEDBACK_LOG_PATH`) — append-only JSONL, при перезапуске не обнуляются.
