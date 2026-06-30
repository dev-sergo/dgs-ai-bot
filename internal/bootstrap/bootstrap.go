// Package bootstrap собирает полностью сконфигурированный *app.App из config.
// Общая проводка для всех точек входа (cmd/server — HTTP, cmd/bot — Telegram),
// чтобы транспорты могли развиваться и деплоиться независимо, не дублируя сборку.
package bootstrap

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"path/filepath"
	"time"

	"dgsbot/internal/advisor"
	"dgsbot/internal/app"
	"dgsbot/internal/config"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/feedback"
	"dgsbot/internal/llm"
	"dgsbot/internal/narrator"
	"dgsbot/internal/planner"
	"dgsbot/internal/querylog"
	"dgsbot/internal/resolver"
	"dgsbot/internal/session"
	"dgsbot/internal/tenantctx"
)

// productIndexTimeout ограничивает фоновое построение индекса товаров: перебор всей
// истории заказов не должен висеть бесконечно при недоступном/медленном API.
const productIndexTimeout = 6 * time.Minute

// reportClient собирает клиент Report-API из конфига Dooglys, выбирая режим
// авторизации. Возвращает nil, если креды выбранного режима не заданы (Report-API
// просто выключен — отчёты идут из fallback'а composite). База: ReportBase, иначе Base.
func reportClient(d config.Dooglys) dooglys.Client {
	rb := d.ReportBase
	if rb == "" {
		rb = d.Base
	}
	// xcontext: явно выбран и задан x-context.
	if d.ReportAuth == string(dooglys.ReportAuthXContext) {
		if d.XContext == "" {
			return nil
		}
		log.Printf("dooglys: Report-API personnel → %s (auth=xcontext)", rb)
		return dooglys.NewReportAPIClientXContext(rb, d.XContext)
	}
	// token (default): нужен access-token; tenant-domain берём из Domain.
	if d.AccessToken == "" {
		// Совместимость: token-режим по умолчанию, но задан только x-context — поднимаем его.
		if d.XContext != "" {
			log.Printf("dooglys: Report-API personnel → %s (auth=xcontext, fallback)", rb)
			return dooglys.NewReportAPIClientXContext(rb, d.XContext)
		}
		return nil
	}
	log.Printf("dooglys: Report-API personnel → %s (auth=token, tenant=%s)", rb, d.Domain)
	return dooglys.NewReportAPIClientToken(rb, d.AccessToken, d.Domain)
}

// App строит app.App из конфига: planner/narrator/advisor, справочник тенантов,
// клиент данных + резолвер, querylog. Возвращает cleanup (закрыть querylog) для defer.
// Фатальные ошибки конфигурации (нет creds, не читается справочник) — как error;
// точка входа сама решает, ронять ли процесс.
func App(cfg config.Config) (*app.App, func(), error) {
	// Планировщик, нарратор, консультант: реальная LLM или детерминированные стабы.
	var pl planner.Planner
	var nar narrator.Narrator
	var adv advisor.Advisor
	switch cfg.PlannerMode {
	case config.PlannerStub:
		pl = planner.NewStub()
		nar = narrator.NewTemplate()
		adv = advisor.NewTemplate()
	default:
		cli := llm.New(cfg.LLM)
		pl = planner.NewLLM(cli, cfg.LLM.Model, cfg.LLM.ForceJSON)
		llmNar := narrator.NewLLM(cli, cfg.LLM.Model)
		llmNar.Logger = slog.Default() // наблюдаемость fallback'а на Compose (срыв qwen)
		nar = llmNar
		adv = advisor.NewLLM(cli, cfg.LLM.Model)
	}

	// Справочник тенантов.
	tenants, err := tenantctx.Load(filepath.Join(cfg.FixturesPath, "tenants.example.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("tenants: %w", err)
	}

	// Источник данных: fixture (по умолчанию, детерминированный) или http (реальный Dooglys).
	// Резолвер имя→uuid: при http берём живые uuid из HTML-формы отчёта, при fixture —
	// офлайн grid-снимки (детерминированный путь CI/eval).
	var client dooglys.Client
	var res *resolver.Store
	switch cfg.Dooglys.Mode {
	case config.DooglysAPI:
		if cfg.Dooglys.Login == "" || cfg.Dooglys.Password == "" {
			return nil, nil, fmt.Errorf("DGS_CLIENT=api requires DGS_LOGIN and DGS_PASSWORD to be set")
		}
		log.Printf("dooglys: using JSON API client → %s (domain=%s); payment+products via API, прочее — фикстуры",
			cfg.Dooglys.Base, cfg.Dooglys.Domain)
		api := dooglys.NewAPIClient(cfg.Dooglys.Base, cfg.Dooglys.Domain, cfg.Dooglys.Login, cfg.Dooglys.Password)
		// Гибрид: payment+products — живой JSON API; paycheck/orders — фикстуры.
		// personnel — Report-API если DGS_XCONTEXT задан, иначе тоже фикстура.
		// payment/products: при доступном Report-API — server-side агрегаты (единый
		// источник = числа админки Dooglys); иначе самосбор APIClient (fallback).
		byReport := map[string]dooglys.Client{"payment": api, "products": api}
		// Report-API: включается, когда заданы креды одного из режимов.
		// token (внешний api.dooglys.com) — primary для демо; xcontext — внутренний (кубы).
		if rc := reportClient(cfg.Dooglys); rc != nil {
			byReport["personnel"] = rc
			byReport["payment"] = rc  // 3a: снять с самосбора → server-side агрегаты
			byReport["products"] = rc // 3a: то же; bonus — profit из Report-API
			log.Printf("dooglys: payment/products → Report-API (server-side агрегаты)")
		}
		client = dooglys.NewComposite(byReport, dooglys.NewFixtureClient(cfg.FixturesPath))
		// Индекс товаров перебирает всю историю заказов (~минуты) — строим в ФОНЕ с
		// таймаутом, чтобы транспорт слушал сразу: до готовности товары резолвятся из
		// фикстур, потом SetOptions атомарно заменяет их живыми (Store под RWMutex).
		res = resolver.Load(cfg.FixturesPath)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), productIndexTimeout)
			defer cancel()
			if opts, err := api.ProductIndex(ctx); err != nil {
				log.Printf("resolver: живой индекс товаров недоступен (%v) — товары из фикстур", err)
			} else {
				res.SetOptions("product", opts)
				log.Printf("resolver: %d товаров из живых заказов (фоновый индекс готов)", len(opts))
			}
		}()
	case config.DooglysHTTP:
		if cfg.Dooglys.Cookie == "" {
			return nil, nil, fmt.Errorf("DGS_CLIENT=http requires DGS_COOKIE to be set")
		}
		log.Printf("dooglys: using HTTP client → %s", cfg.Dooglys.Base)
		hc := dooglys.NewHTMLClient(cfg.Dooglys.Base, cfg.Dooglys.Cookie)
		client = hc
		if live, err := resolver.NewLiveStore(context.Background(), hc); err != nil {
			log.Printf("resolver: live store unavailable (%v) — fallback to fixtures", err)
			res = resolver.Load(cfg.FixturesPath)
		} else {
			log.Printf("resolver: using live UUIDs from Dooglys HTML form")
			res = live
		}
	default:
		client = dooglys.NewFixtureClient(cfg.FixturesPath)
		res = resolver.Load(cfg.FixturesPath)
	}

	a := app.New(pl, tenants, client, res, nar, adv, session.NewStore())

	// Датасет вопросов/ответов (JSONL) — для продуктовой аналитики и дообучения.
	// Включается, только если задан QUERY_LOG_PATH; файл переживает рестарт (append).
	// Открыть не удалось (напр. нет прав на смонтированный каталог) — это НЕ повод
	// ронять сервис: предупреждаем и работаем без датасета (лог просто выключен).
	cleanup := func() {}
	if cfg.QueryLogPath != "" {
		if ql, err := querylog.Open(cfg.QueryLogPath); err != nil {
			log.Printf("WARNING: query log отключён — не открыть %s: %v "+
				"(проверь права на каталог: chown 65532:65532)", cfg.QueryLogPath, err)
		} else {
			a.QueryLog = ql
			prev := cleanup
			cleanup = func() { prev(); ql.Close() }
			log.Printf("query log: пишем датасет вопрос→план→ответ → %s", cfg.QueryLogPath)
		}
	}
	if cfg.FeedbackLogPath != "" {
		if fl, err := feedback.Open(cfg.FeedbackLogPath); err != nil {
			log.Printf("WARNING: feedback log отключён — не открыть %s: %v", cfg.FeedbackLogPath, err)
		} else {
			a.FeedbackLog = fl
			prev := cleanup
			cleanup = func() { prev(); fl.Close() }
			log.Printf("feedback log: пишем оценки → %s", cfg.FeedbackLogPath)
		}
	}

	return a, cleanup, nil
}
