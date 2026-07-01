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

// tenantDooglys накладывает пер-тенантные креды (Domain/AccessToken/XContext) поверх
// общих настроек Dooglys (Mode/Base/ReportBase/ReportAuth/Login/Password). Пустой
// пер-тенантный кред НЕ затирает общий (поддержка «токен общий, различие — в домене»).
func tenantDooglys(base config.Dooglys, tc config.TenantConfig) config.Dooglys {
	d := base
	if tc.Domain != "" {
		d.Domain = tc.Domain
	}
	if tc.AccessToken != "" {
		d.AccessToken = tc.AccessToken
	}
	if tc.XContext != "" {
		d.XContext = tc.XContext
	}
	return d
}

// tenantData собирает источник данных и резолвер ОДНОГО тенанта по его кредам.
// Источник: fixture (детерминированный, без сети) / http (SSR-HTML) / api (JSON+Report-API).
// Каждый тенант получает свой экземпляр — это и есть граница изоляции данных.
func tenantData(cfg config.Config, tc config.TenantConfig) (dooglys.Client, *resolver.Store) {
	d := tenantDooglys(cfg.Dooglys, tc)
	switch d.Mode {
	case config.DooglysAPI:
		if d.Login == "" || d.Password == "" {
			// JSON API самосбора нет — но Report-API (token/xcontext) может работать сам.
			// Не роняем тенанта: без login/password payment/products возьмёт Report-API
			// (если есть креды) либо фикстуры. personnel — Report-API/фикстуры.
			log.Printf("dooglys[%s]: DGS_LOGIN/PASSWORD не заданы — JSON API самосбор выключен", tc.ID)
		}
		byReport := map[string]dooglys.Client{}
		var api *dooglys.APIClient
		if d.Login != "" && d.Password != "" {
			log.Printf("dooglys[%s]: JSON API → %s (domain=%s); payment+products via API", tc.ID, d.Base, d.Domain)
			api = dooglys.NewAPIClient(d.Base, d.Domain, d.Login, d.Password)
			byReport["payment"] = api
			byReport["products"] = api
		}
		// Report-API: server-side агрегаты (единый источник = числа админки Dooglys).
		// token (внешний api.dooglys.com) — primary демо; xcontext — внутренний (кубы).
		if rc := reportClient(d); rc != nil {
			byReport["personnel"] = rc
			byReport["payment"] = rc  // снять с самосбора → server-side агрегаты
			byReport["products"] = rc // то же; bonus — profit из Report-API
			log.Printf("dooglys[%s]: payment/products/personnel → Report-API", tc.ID)
		}
		client := dooglys.NewComposite(byReport, dooglys.NewFixtureClient(cfg.FixturesPath))
		res := resolver.Load(cfg.FixturesPath)
		// Индекс товаров перебирает всю историю заказов (~минуты) — строим в ФОНЕ с
		// таймаутом, чтобы транспорт слушал сразу: до готовности товары резолвятся из
		// фикстур, потом SetOptions атомарно заменяет их живыми (Store под RWMutex).
		if api != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), productIndexTimeout)
				defer cancel()
				if opts, err := api.ProductIndex(ctx); err != nil {
					log.Printf("resolver[%s]: живой индекс товаров недоступен (%v) — из фикстур", tc.ID, err)
				} else {
					res.SetOptions("product", opts)
					log.Printf("resolver[%s]: %d товаров из живых заказов (фон готов)", tc.ID, len(opts))
				}
			}()
		}
		return client, res
	case config.DooglysHTTP:
		log.Printf("dooglys[%s]: HTTP client → %s", tc.ID, d.Base)
		hc := dooglys.NewHTMLClient(d.Base, d.Cookie)
		if live, err := resolver.NewLiveStore(context.Background(), hc); err != nil {
			log.Printf("resolver[%s]: live store unavailable (%v) — fallback to fixtures", tc.ID, err)
			return hc, resolver.Load(cfg.FixturesPath)
		} else {
			log.Printf("resolver[%s]: using live UUIDs from Dooglys HTML form", tc.ID)
			return hc, live
		}
	default:
		return dooglys.NewFixtureClient(cfg.FixturesPath), resolver.Load(cfg.FixturesPath)
	}
}

// App строит app.App из конфига: planner/narrator/advisor, справочник тенантов,
// и пер-тенантный реестр {client, resolver} (по одному набору на тенанта из cfg.Tenants).
// Возвращает cleanup (закрыть querylog) для defer. Фатальные ошибки конфигурации (битая
// авторизация, не читается справочник) — как error; точка входа сама решает, ронять ли процесс.
func App(cfg config.Config) (*app.App, func(), error) {
	// Ранняя валидация авторизации: битый конфиг падает на старте с внятным сообщением,
	// а не HTTP 500 на первом запросе (docs/12 §8).
	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}

	// Планировщик, нарратор, консультант: реальная LLM или детерминированные стабы.
	// Тенант-агностичны и общие для всех тенантов (один LLM-клиент, одна сессия-стора).
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

	// Справочник тенантов (таймзона/валюта/точки) — общий, ключуется по tenant_id/domain.
	tenants, err := tenantctx.Load(filepath.Join(cfg.FixturesPath, "tenants.example.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("tenants: %w", err)
	}

	a := app.NewMulti(pl, tenants, nar, adv, session.NewStore())

	// Пер-тенантный реестр данных: у каждого тенанта СВОЙ client+resolver (граница
	// изоляции). В fixture-режиме наборы идентичны (детерминированный CI); изоляция по
	// реальным данным возникает в api-режиме, где у каждого свои домен/токен.
	for _, tc := range cfg.Tenants {
		client, res := tenantData(cfg, tc)
		a.Register(tc.ID, client, res)
	}

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
