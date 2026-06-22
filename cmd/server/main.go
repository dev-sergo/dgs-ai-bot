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

	// Планировщик и нарратор: реальная LLM или детерминированные стабы.
	var pl planner.Planner
	var nar narrator.Narrator
	switch cfg.PlannerMode {
	case config.PlannerStub:
		pl = planner.NewStub()
		nar = narrator.NewTemplate()
	default:
		cli := llm.New(cfg.LLM)
		pl = planner.NewLLM(cli, cfg.LLM.Model, cfg.LLM.ForceJSON)
		nar = narrator.NewLLM(cli, cfg.LLM.Model)
	}

	// Справочник тенантов.
	tenants, err := tenantctx.Load(filepath.Join(cfg.FixturesPath, "tenants.example.json"))
	if err != nil {
		log.Fatalf("tenants: %v", err)
	}

	// Источник данных: fixture (по умолчанию, детерминированный) или http (реальный Dooglys).
	// Переключается через DGS_CLIENT=http + DGS_BASE + DGS_COOKIE.
	var client dooglys.Client
	if cfg.Dooglys.Mode == config.DooglysHTTP {
		if cfg.Dooglys.Cookie == "" {
			log.Fatal("DGS_CLIENT=http requires DGS_COOKIE to be set")
		}
		log.Printf("dooglys: using HTTP client → %s", cfg.Dooglys.Base)
		client = dooglys.NewHTMLClient(cfg.Dooglys.Base, cfg.Dooglys.Cookie)
	} else {
		client = dooglys.NewFixtureClient(cfg.FixturesPath)
	}

	// Справочники для резолва имён в uuid.
	res := resolver.Load(cfg.FixturesPath)

	a := app.New(pl, tenants, client, res, nar, session.NewStore())
	srv := httpx.New(cfg, a)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
