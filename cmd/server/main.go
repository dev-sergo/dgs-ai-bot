// Command server — точка входа Dooglys AI-bot.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "time/tzdata" // встроенная база таймзон — чтобы работало в distroless без системного tzdata

	"dgsbot/internal/advisor"
	"dgsbot/internal/app"
	"dgsbot/internal/config"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/llm"
	"dgsbot/internal/narrator"
	"dgsbot/internal/planner"
	"dgsbot/internal/querylog"
	"dgsbot/internal/resolver"
	"dgsbot/internal/session"
	"dgsbot/internal/tenantctx"
	httpx "dgsbot/internal/transport/http"
	tgx "dgsbot/internal/transport/telegram"
)

// productIndexTimeout ограничивает фоновое построение индекса товаров: перебор всей
// истории заказов не должен висеть бесконечно при недоступном/медленном API.
const productIndexTimeout = 6 * time.Minute

func main() {
	cfg := config.Load()
	log.Printf("config: %s", cfg.Summary())

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
		nar = narrator.NewLLM(cli, cfg.LLM.Model)
		adv = advisor.NewLLM(cli, cfg.LLM.Model)
	}

	// Справочник тенантов.
	tenants, err := tenantctx.Load(filepath.Join(cfg.FixturesPath, "tenants.example.json"))
	if err != nil {
		log.Fatalf("tenants: %v", err)
	}

	// Источник данных: fixture (по умолчанию, детерминированный) или http (реальный Dooglys).
	// Переключается через DGS_CLIENT=http + DGS_BASE + DGS_COOKIE.
	// Резолвер имя→uuid: при http берём живые uuid из HTML-формы отчёта, при fixture —
	// офлайн grid-снимки (детерминированный путь CI/eval).
	var client dooglys.Client
	var res *resolver.Store
	switch cfg.Dooglys.Mode {
	case config.DooglysAPI:
		if cfg.Dooglys.Login == "" || cfg.Dooglys.Password == "" {
			log.Fatal("DGS_CLIENT=api requires DGS_LOGIN and DGS_PASSWORD to be set")
		}
		log.Printf("dooglys: using JSON API client → %s (domain=%s); payment+products via API, прочее — фикстуры",
			cfg.Dooglys.Base, cfg.Dooglys.Domain)
		api := dooglys.NewAPIClient(cfg.Dooglys.Base, cfg.Dooglys.Domain, cfg.Dooglys.Login, cfg.Dooglys.Password)
		// Гибрид: payment + products — живой API (из тех же заказов/order_items),
		// paycheck/orders — фикстуры (ещё не на API).
		client = dooglys.NewComposite(
			map[string]dooglys.Client{"payment": api, "products": api},
			dooglys.NewFixtureClient(cfg.FixturesPath),
		)
		// Резолвер: sale_point/user — из фикстур, товары — из живых заказов (имена совпадают
		// с тем, что в отчёте → drill-down по товару резолвится). Индекс товаров перебирает
		// всю историю заказов (~минуты) — строим его в ФОНЕ с таймаутом, чтобы HTTP слушал
		// сразу: до готовности индекса товары резолвятся из фикстур, потом SetOptions
		// атомарно заменяет их живыми (Store под RWMutex). Раньше синхронный вызов держал
		// старт ~4 мин → каждый редеплой = минуты «502».
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
			log.Fatal("DGS_CLIENT=http requires DGS_COOKIE to be set")
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
	if cfg.QueryLogPath != "" {
		if ql, err := querylog.Open(cfg.QueryLogPath); err != nil {
			log.Printf("WARNING: query log отключён — не открыть %s: %v "+
				"(проверь права на каталог: chown 65532:65532)", cfg.QueryLogPath, err)
		} else {
			defer ql.Close()
			a.QueryLog = ql
			log.Printf("query log: пишем датасет вопрос→план→ответ → %s", cfg.QueryLogPath)
		}
	}

	srv := httpx.New(cfg, a)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Telegram-бот запускается параллельно с HTTP, если задан токен.
	// Пустой TELEGRAM_TOKEN → только HTTP, поведение идентично текущему.
	if cfg.Telegram.Token != "" {
		bot, err := tgx.New(cfg.Telegram, a)
		if err != nil {
			log.Fatalf("telegram: %v", err)
		}
		go bot.Run(ctx)
	}

	if err := srv.Run(ctx); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
