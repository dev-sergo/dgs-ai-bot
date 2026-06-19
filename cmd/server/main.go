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
		pl = planner.NewLLM(cli, cfg.LLM.Model)
		nar = narrator.NewLLM(cli, cfg.LLM.Model)
	}

	// Справочник тенантов.
	tenants, err := tenantctx.Load(filepath.Join(cfg.FixturesPath, "tenants.example.json"))
	if err != nil {
		log.Fatalf("tenants: %v", err)
	}

	// Источник данных: фикстуры (M1). На M4 заменится на реальный клиент за тем же интерфейсом.
	client := dooglys.NewFixtureClient(cfg.FixturesPath)

	// Справочники для резолва имён в uuid.
	res := resolver.Load(cfg.FixturesPath)

	a := app.New(pl, tenants, client, res, nar)
	srv := httpx.New(cfg, a)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
