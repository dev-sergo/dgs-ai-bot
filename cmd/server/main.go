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
	"dgsbot/internal/planner"
	"dgsbot/internal/tenantctx"
	httpx "dgsbot/internal/transport/http"
)

func main() {
	cfg := config.Load()
	log.Printf("config: %s", cfg.Summary())

	// Планировщик: реальная LLM или детерминированный стаб.
	var pl planner.Planner
	switch cfg.PlannerMode {
	case config.PlannerStub:
		pl = planner.NewStub()
	default:
		pl = planner.NewLLM(llm.New(cfg.LLM), cfg.LLM.Model)
	}

	// Справочник тенантов.
	tenants, err := tenantctx.Load(filepath.Join(cfg.FixturesPath, "tenants.example.json"))
	if err != nil {
		log.Fatalf("tenants: %v", err)
	}

	// Источник данных: фикстуры (M1). На M4 заменится на реальный клиент за тем же интерфейсом.
	client := dooglys.NewFixtureClient(cfg.FixturesPath)

	a := app.New(pl, tenants, client)
	srv := httpx.New(cfg, a)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
