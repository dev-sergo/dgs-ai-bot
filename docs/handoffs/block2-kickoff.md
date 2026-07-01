# СТАТУС: ✅ ЗАКРЫТ (2026-07-01, в main) — промт-хэндофф для истории

Итог: реестр {client, resolver} на тенанта внутри одного App (не N App); 3 бота,
whitelist на бот, строгая изоляция (неизвестный тенант отказан, не подставляет чужой
источник), сессии скоупятся `tg:<tenant>:<chatID>`, валидация конфига на старте.
Коммиты 6a6895f, 5aa4abb, 6a52310, ff9a5ff. `make test`/`make vet` зелёные, изоляционные
тесты ассертят A≠B / tenant-из-привязки / unknown→refused.

---

# Задача: Блок 2 (мультитенантность — 3 бота) MVP Telegram-бота Dooglys

Контекст фазы и план — `docs/12-mvp-tz-and-plan.md` (раздел 7, Блок 2 + раздел 8
про валидацию конфига) и память проекта `active-dev-plan`. Дедлайн фазы — пятница.
Блок 1 (данные) ЗАКРЫТ и в main. Блок 2 — структурный, РИГ НЕ НУЖЕН.

## Цель
3 отдельных Telegram-бота (по одному на тенанта), каждый привязан к своему тенанту,
со своим whitelist, с полной изоляцией данных. Заложить шов, чтобы позже дёшево свести
к одному боту на все тенанты.

## Топология (подтверждена с заказчиком)
- 3 разных бота через @BotFather → 3 токена. Каждый бот жёстко на одного тенанта.
- Whitelist по chat_id на каждом боте.
- НО код проектировать так, чтобы «1 бот на все тенанты» был сменой резолвера, а не
  переписыванием ядра/клиента.

## Текущее состояние (проверено в коде)
- `internal/app/app.go:67` — `App` держит ОДИН `client dooglys.Client`; `a.client.Fetch`
  зовётся по всему пайплайну. Это главный шов: клиент должен резолвиться ПО tenantID.
- `App` также держит `resolver *resolver.Store` и `tenants *tenantctx.Store` — оба
  тенант-специфичны. ИНСПЕКТИРУЙ `internal/resolver`, `internal/tenantctx`,
  `internal/bootstrap` — реши: реестр {client, resolver} на тенант внутри одного App,
  ИЛИ N экземпляров App. Обоснуй в коммите.
- `app.Ask(ctx, tenantID, sessionID, text)` УЖЕ принимает tenantID — НЕ убирать.
- Конфиг ОДНО-тенантный: `config.Telegram{Token, Allowlist, DefaultTenant}` и один блок
  `config.Dooglys`. Нужно → список тенантов.
- Фабрика клиентов ГОТОВА (Блок 1): `NewReportAPIClientToken(base, accessToken,
  tenantDomain)` и `NewReportAPIClientXContext(base, xctx)` — из аргументов.
- Telegram: `internal/transport/telegram/bot.go` — polling + allowlist-guard + 👍/👎.

## Задачи

### T5. Мультитенантный конфиг
- Список тенантов `[]TenantConfig{ID, BotToken, Allowlist, Domain/AccessToken(|XContext)}`.
  Формат ENV — на выбор (JSON в `TENANTS`, или `TENANT_<k>_*`), задокументируй.
- Пустой список → одно-тенантная деградация из legacy. `Summary()` печатает тенантов
  БЕЗ секретов.

### T6. Реестр тенантов + запуск 3 ботов (главный шов)
- Шов `resolveTenant(chatID) -> tenantID` + реестр `tenantID -> {client, resolver, allowlist}`.
- App достаёт client/resolver ПО tenantID из реестра.
- Bootstrap: N клиентов фабрикой Блока 1; поднять 3 бота (по токену).
- Готово когда: 3 бота отвечают, каждый своим тенантом, на боевых данных.

### T7. Whitelist на тенанта
- Связать allowlist из bot.go с конкретным тенантом. Тест: чужой chat_id отбит.

### T8. Тест изоляции (детерминированный, без рига)
- Доказать: запрос тенанта A НЕ достаёт данные тенанта B (разные stub-клиенты).
- tenantID из привязки бота, не из ввода пользователя. Незарегистрированный → отказ.

### T9. Валидация конфига на старте (из docs/12 §8)
- `DGS_REPORT_AUTH=token` без `AccessToken` → ранний fail; тенант без токена бота → fail.

## Правила
- Сборка/тесты — docker: `make test`, `make vet` (go локально нет). Рига НЕ нужно.
- Коммиты EN без Co-Authored-By, раздельные на задачу. Sandwich: tenantID/секреты не в LLM.
- НЕ трогать промпт планировщика/refuse-regex (P1). Прогоны — через `tee`.

## От пользователя нужно
- 3 токена ботов из @BotFather. 3 тенанта: `tenant-domain` + `access-token` (или что токен
  общий, различие в домене). Разрешённые chat_id на бота.
