// Command server — точка входа Dooglys AI-bot.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	_ "time/tzdata" // встроенная база таймзон — чтобы работало в distroless без системного tzdata

	"dgsbot/internal/advisor"
	"dgsbot/internal/app"
	"dgsbot/internal/config"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/llm"
	"dgsbot/internal/narrator"
	"dgsbot/internal/planner"
	"dgsbot/internal/resolver"
	"dgsbot/internal/session"
	"dgsbot/internal/tenantctx"
	httpx "dgsbot/internal/transport/http"
)

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
		log.Printf("dooglys: using JSON API client → %s (domain=%s)", cfg.Dooglys.Base, cfg.Dooglys.Domain)
		client = dooglys.NewAPIClient(cfg.Dooglys.Base, cfg.Dooglys.Domain, cfg.Dooglys.Login, cfg.Dooglys.Password)
		// Payment-данные идут из API; резолвер имя→uuid пока из офлайн grid-снимков.
		res = resolver.Load(cfg.FixturesPath)
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
	srv := httpx.New(cfg, a)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
