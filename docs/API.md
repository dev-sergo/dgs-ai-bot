# HTTP API — Dooglys AI-bot

Бэкенд слушает на `HTTP_ADDR` (дефолт `:8088`). В docker-compose доступен через nginx на `:8090` по пути `/api/*`.

---

## Аутентификация

Если `AUTH_TOKEN` задан — все endpoints (кроме `/healthz`) требуют токен.
Токен передаётся одним из способов (приоритет слева направо):

```
X-Auth-Token: <token>
Authorization: Bearer <token>
?key=<token>
```

При неверном токене: `HTTP 403`, JSON `{"error": "..."}`.

---

## POST /ask

Задать вопрос. Основной endpoint.

**Заголовки:**
```
Content-Type: application/json
X-Tenant-ID: <tenant_id>      (опционально, можно в теле)
X-Session-ID: <session_id>    (опционально, для диалогового контекста)
```

**Тело запроса:**
```json
{
  "query": "выручка за вчера",
  "tenant_id": "mock_single",
  "session_id": "user-42"
}
```

| Поле | Тип | Описание |
|---|---|---|
| `query` | string | Вопрос на естественном языке (RU) |
| `tenant_id` | string | ID тенанта; дефолт `mock_single` |
| `session_id` | string | Ключ сессии для контекста диалога; дефолт `default:<tenant_id>` |

**Ответ `200 OK`:**
```json
{
  "text": "Выручка за вчера составила 45 320 ₽...",
  "envelope": {
    "type": "plain",
    "tenant_id": "mock_single",
    "period": { "from": "25.06.2026", "to": "25.06.2026", "tz": "Europe/Moscow" },
    "currency": "RUB",
    "columns": [
      { "key": "date", "label": "Дата", "unit": "date" },
      { "key": "revenue", "label": "Выручка", "unit": "RUB" }
    ],
    "rows": [
      { "date": "25.06.2026", "revenue": 45320.0 }
    ],
    "summary": { "revenue": 45320.0 },
    "narrative": "Выручка за вчера составила 45 320 ₽."
  },
  "clarify": false,
  "need_clarify": false
}
```

| Поле | Описание |
|---|---|
| `text` | Готовый ответ для отображения пользователю |
| `envelope` | Структурированные данные (числа, таблица) — источник истины |
| `envelope.type` | Метод расчёта: `plain` / `compare` / `contribution` / `top_n` / `channel_share` / `forecast` |
| `envelope.summary` | Агрегаты периода (числа float64 по ключам-метрикам) |
| `envelope.rows` | Строки таблицы ([]map[string]any) |
| `clarify` | `true` — бот просит уточнения (вопрос непонятен) |
| `need_clarify` | `true` — бот переспрашивает подтверждение плана (confidence 0.5–0.7) |

**Ошибки:**
```json
HTTP 400  {"error": "..."}    // невалидный JSON
HTTP 403  {"error": "..."}    // неверный токен
HTTP 500  {"error": "..."}    // внутренняя ошибка
```

---

## GET /export

Скачать ответ в Excel (`.xlsx`). Прогоняет тот же запрос, что и `/ask`, но возвращает файл.

**Query параметры:**
```
text=<вопрос>
tenant_id=<tenant_id>   (опционально)
session_id=<session_id> (опционально)
key=<token>             (если AUTH_TOKEN задан)
```

**Ответ:** `Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`
Файл с именем вида `report_payment_2026-06-25.xlsx`.

---

## POST /feedback

Записать оценку ответа (👍/👎).

**Тело:**
```json
{
  "answer_id": "abc123",
  "rating": 1
}
```

| Поле | Тип | Описание |
|---|---|---|
| `answer_id` | string | ID ответа из `/ask` |
| `rating` | int | `1` = лайк, `-1` = дизлайк |

Записывается в `FEEDBACK_LOG_PATH` (JSONL). Если путь не задан — endpoint отвечает `200 OK` но ничего не пишет.

**Ответ:** `HTTP 200`, пустое тело.

---

## GET /healthz

Healthcheck. Не требует токена. Всегда возвращает `HTTP 200`.

```json
{"status": "ok"}
```

---

## Мультитенантность

`tenant_id` определяет, какие данные видит пользователь. Определяется из (приоритет):
1. Заголовок `X-Tenant-ID`
2. Поле `tenant_id` в JSON-теле
3. Дефолт: `mock_single`

`tenant_id` **никогда** не передаётся в LLM — только server-side.

---

## Диалоговый контекст

`session_id` привязывает запрос к сессии. В рамках одной сессии бот запоминает последний план
(отчёт + период + фильтры) и применяет его к follow-up вопросам («а за прошлый?», «а по карте?»).

Сессии хранятся in-memory. После перезапуска сервиса — сбрасываются.
