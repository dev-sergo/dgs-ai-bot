# СТАТУС: ✅ ЗАКРЫТ (2026-07-01, в main) — промт-хэндофф для истории

Итог: внешняя token-авторизация Report-API, sort_by-дженерик, 6 отчётов ТЗ +
9 транспортно-готовых, payment/products сняты с самосбора. Коммиты 41e904a, 17e1028,
3fe7d21. Live-проверено на api.dooglys.com, 0 регресса планировщика.

---

# Задача: Блок 1 (данные/фундамент) MVP Telegram-бота Dooglys

Контекст фазы и полное ТЗ — в `docs/12-mvp-tz-and-plan.md` (раздел 7, Блок 1) и в
памяти проекта `active-dev-plan`. Дедлайн фазы — пятница. Блок 1: подключить 6 отчётов
ТЗ к боевому Report-API Dooglys.

## Что уже есть (стартовая точка, проверено в коде)
- `internal/dooglys/reportclient.go` — `ReportAPIClient`: пагинация + x-context auth,
  НО `reportPathMap` содержит только `"personnel"`, а `Fetch` шлёт лишь
  `date_from/date_to/per_page/page` (sort_by НЕ отправляет).
- `internal/dooglys/apiclient.go` — `APIClient` (token-auth): payment/products идут
  самосбором из сырых заказов. Это надо снять и перевести на Report-API.
- `internal/config/config.go` — `Dooglys` имеет `ReportBase` + `XContext`, но НЕТ поля
  `access-token` и нет выбора режима авторизации.
- Тесты пакета: `reportclient_test.go`, `apiclient_golden_test.go`, `contract_test.go`.
- Каталог отчётов — `internal/catalog`; нарратор — `internal/narrator`.

## Контракт авторизации (два режима, primary для демо — ВНЕШНИЙ)
Внешний (api.dooglys.com), для демо:
```
GET https://api.dooglys.com/api/v1/reports/report/payment?date_from=2026-06-01&date_to=2026-06-30&sort_by=date&sort_order=asc
access-token: <token>
tenant-domain: rukagreka
```
Внутренний (в кубах, оставить как второй режим):
```
GET http://report.default.svc/api/v1/report/payment?...
x-context: {"tenant_id":"...","tenant_domain":"..."}
```
Отличия: внешний путь имеет префикс `/reports`, внутренний — нет; внешний —
заголовки `access-token`+`tenant-domain`, внутренний — `x-context`.
Полная спека всех методов — `docs/report.yml`.

## Задачи Блока 1 (последовательно)

### 1. Внешняя авторизация + конфиг
- Добавить в `config.Dooglys`: `AccessToken` (`DGS_ACCESS_TOKEN`), `ReportAuth`
  (`DGS_REPORT_AUTH` = `token` | `xcontext`, default `token`). `DGS_REPORT_BASE` для
  внешнего = `https://api.dooglys.com/api/v1/reports`, для внутреннего =
  `http://report.default.svc/api/v1`.
- В `ReportAPIClient` поддержать оба режима: `token` шлёт `access-token`+`tenant-domain`,
  `xcontext` — `x-context` (как сейчас). Не ломать существующий x-context-путь.
- `config.Summary()` не должен печатать секреты (access-token/x-context).
- Готово когда: Go-клиент тянет `/report/payment` с боевым токеном (эквивалент curl).

### 2. sort_by/sort_order + дженерик-транспорт
- В `report.yml` у КАЖДОГО отчёта `sort_by: required` со своим enum. Сейчас клиент его
  не шлёт → боевой API вернёт HTTP 400.
- Ввести `reportDefaultSort` (map отчёт → валидный `sort_by`), `sort_order` по умолчанию
  `asc`. Дефолты брать из enum'ов `report.yml`.
- Готово когда: personnel/payment не падают с 400 на боевом API.

### 3. Path-map: 6 отчётов ТЗ + ~10 уровня B; снять payment/products с самосбора
- В `reportPathMap`/`reportLabel` добавить 6 отчётов ТЗ: payment, source-order,
  products, categories, personnel, cash-on-hand, cash-income-outcome.
- Прописать ~10 GET-отчётов уровня B транспортно (path + sort-дефолт), но НЕ роутить их
  планировщиком.
- Перевести payment/products с самосбора `APIClient` на Report-API. НЕ роутить уровень C
  (rfm/abc/customer-*) — у них POST/обяз. параметры.
- Готово когда: 6 отчётов резолвятся через Report-API, числа совпадают с фикстурами.

### 4. Каталог + нарратор + фильтры для 6 отчётов
- Прописать 6 отчётов в `internal/catalog` и в нарраторе. Заполнить `reportFilterColumn`.
- Готово когда: бот текстом отвечает на 6 типов вопросов на боевых данных.

## Гарантия совместимости с будущим швом мультитенантности (Блок 2)
- `ReportAPIClient` строить ТОЛЬКО из явных аргументов конструктора. НЕ читать глобальный
  `config` изнутри клиента и НЕ делать его синглтоном. Конструктор пригоден к вызову N раз.
- `app.Ask(ctx, tenantID, ...)` уже принимает tenantID — НЕ убирать этот параметр.

## Правила
- Боевые прогоны (real LLM / реальный API) запускает ТОЛЬКО пользователь; я готовлю
  детерминированный «Тир 0» (юнит/golden/contract) и отдаю команду.
- Правки промпта планировщика → полный eval до коммита. Коммиты EN без Co-Authored-By.
- Прогоны сохранять через `tee <name>-$(date +%Y%m%d-%H%M).log`. Sandwich: секреты не в LLM.
